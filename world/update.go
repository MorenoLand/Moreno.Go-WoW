package world

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

type UpdateType uint8

const (
	UpdateValues UpdateType = iota
	UpdateMovementBlock
	UpdateCreate
	UpdateCreate2
	UpdateOutOfRange
	UpdateNearObjects
)

type ObjectType uint8

const (
	ObjectTypeObject ObjectType = iota
	ObjectTypeItem
	ObjectTypeContainer
	ObjectTypeUnit
	ObjectTypePlayer
	ObjectTypeGameObject
	ObjectTypeDynamicObject
	ObjectTypeCorpse
)

const (
	UpdateFlagSelf               uint16 = 0x0001
	UpdateFlagTransport          uint16 = 0x0002
	UpdateFlagHasTarget          uint16 = 0x0004
	UpdateFlagUnknown            uint16 = 0x0008
	UpdateFlagLowGUID            uint16 = 0x0010
	UpdateFlagLiving             uint16 = 0x0020
	UpdateFlagStationaryPosition uint16 = 0x0040
	UpdateFlagVehicle            uint16 = 0x0080
	UpdateFlagPosition           uint16 = 0x0100
	UpdateFlagRotation           uint16 = 0x0200
	MovementFlagForward          uint32 = 0x00000001
	MovementFlagBackward         uint32 = 0x00000002
	MovementFlagStrafeLeft       uint32 = 0x00000004
	MovementFlagStrafeRight      uint32 = 0x00000008
	MovementFlagWalking          uint32 = 0x00000100
	MovementFlagOnTransport      uint32 = 0x00000200
	MovementFlagSwimming         uint32 = 0x00200000
	MovementFlagFlying           uint32 = 0x02000000
	MovementFlagFalling          uint32 = 0x00001000
	MovementFlagSplineElevation  uint32 = 0x04000000
	MovementFlagSplineEnabled    uint32 = 0x08000000
	MovementFlag2Pitching        uint16 = 0x0010
	MovementFlag2Interpolated    uint16 = 0x0400
	SplineFlagFinalPoint         uint32 = 0x00008000
	SplineFlagFinalTarget        uint32 = 0x00010000
	SplineFlagFinalAngle         uint32 = 0x00020000
)

const (
	ObjectScaleField         uint16 = 0x04
	UnitBytes0Field          uint16 = 0x17
	UnitDisplayIDField       uint16 = 0x43
	UnitNativeDisplayIDField uint16 = 0x44
)

type UpdateMovement struct {
	Flags         uint16
	MovementFlags uint32
	Position      WorldPosition
	HasPosition   bool
	Speeds        [9]float32
}

type UpdateBlock struct {
	Type       UpdateType
	GUID       uint64
	ObjectType ObjectType
	Spawned    bool
	Movement   UpdateMovement
	Fields     map[uint16]uint32
	GUIDs      []uint64
}

func ReadPackedGUID(r *Reader) (uint64, error) {
	mask, err := r.U8()
	if err != nil {
		return 0, err
	}
	var guid uint64
	for index := 0; index < 8; index++ {
		if mask&(1<<uint(index)) == 0 {
			continue
		}
		value, readErr := r.U8()
		if readErr != nil {
			return 0, readErr
		}
		guid |= uint64(value) << uint(index*8)
	}
	return guid, nil
}

func readUpdateFields(r *Reader) (map[uint16]uint32, error) {
	count, err := r.U8()
	if err != nil {
		return nil, err
	}
	masks := make([]uint32, count)
	for index := range masks {
		if masks[index], err = r.U32(); err != nil {
			return nil, err
		}
	}
	fields := make(map[uint16]uint32)
	for word, mask := range masks {
		for bit := uint(0); bit < 32; bit++ {
			if mask&(1<<bit) == 0 {
				continue
			}
			value, readErr := r.U32()
			if readErr != nil {
				return nil, readErr
			}
			fields[uint16(word*32)+uint16(bit)] = value
		}
	}
	return fields, nil
}

func readUpdateMovement(r *Reader) (UpdateMovement, error) {
	flags, err := r.U16()
	if err != nil {
		return UpdateMovement{}, err
	}
	result := UpdateMovement{Flags: flags}
	if flags&UpdateFlagLiving != 0 {
		movementFlags, readErr := r.U32()
		if readErr != nil {
			return UpdateMovement{}, readErr
		}
		result.MovementFlags = movementFlags
		movementFlags2, readErr := r.U16()
		if readErr != nil {
			return UpdateMovement{}, readErr
		}
		if _, err = r.U32(); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.X, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.Y, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.Z, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.Orientation, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		result.HasPosition = true
		if movementFlags&MovementFlagOnTransport != 0 {
			if _, err = ReadPackedGUID(r); err != nil {
				return UpdateMovement{}, err
			}
			if err = r.Skip(16); err != nil {
				return UpdateMovement{}, err
			}
			if err = r.Skip(4 + 1); err != nil {
				return UpdateMovement{}, err
			}
			if movementFlags2&MovementFlag2Interpolated != 0 {
				if err = r.Skip(4); err != nil {
					return UpdateMovement{}, err
				}
			}
		}
		if movementFlags&(MovementFlagSwimming|MovementFlagFlying) != 0 {
			if err = r.Skip(4); err != nil {
				return UpdateMovement{}, err
			}
		}
		if err = r.Skip(4); err != nil {
			return UpdateMovement{}, err
		}
		if movementFlags&MovementFlagFalling != 0 {
			if err = r.Skip(16); err != nil {
				return UpdateMovement{}, err
			}
		}
		if movementFlags&MovementFlagSplineElevation != 0 {
			if err = r.Skip(4); err != nil {
				return UpdateMovement{}, err
			}
		}
		for index := range result.Speeds {
			if result.Speeds[index], err = r.F32(); err != nil {
				return UpdateMovement{}, err
			}
		}
		if movementFlags&MovementFlagSplineEnabled != 0 {
			if err = skipUpdateSpline(r); err != nil {
				return UpdateMovement{}, err
			}
		}
	} else if flags&UpdateFlagPosition != 0 {
		if _, err = ReadPackedGUID(r); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.X, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.Y, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.Z, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if err = r.Skip(12); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.Orientation, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if err = r.Skip(4); err != nil {
			return UpdateMovement{}, err
		}
		result.HasPosition = true
	} else if flags&UpdateFlagStationaryPosition != 0 {
		if result.Position.X, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.Y, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.Z, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		if result.Position.Orientation, err = r.F32(); err != nil {
			return UpdateMovement{}, err
		}
		result.HasPosition = true
	}
	if flags&UpdateFlagUnknown != 0 {
		if err = r.Skip(4); err != nil {
			return UpdateMovement{}, err
		}
	}
	if flags&UpdateFlagLowGUID != 0 {
		if err = r.Skip(4); err != nil {
			return UpdateMovement{}, err
		}
	}
	if flags&UpdateFlagHasTarget != 0 {
		if _, err = ReadPackedGUID(r); err != nil {
			return UpdateMovement{}, err
		}
	}
	if flags&UpdateFlagTransport != 0 {
		if err = r.Skip(4); err != nil {
			return UpdateMovement{}, err
		}
	}
	if flags&UpdateFlagVehicle != 0 {
		if err = r.Skip(8); err != nil {
			return UpdateMovement{}, err
		}
	}
	if flags&UpdateFlagRotation != 0 {
		if err = r.Skip(8); err != nil {
			return UpdateMovement{}, err
		}
	}
	return result, nil
}

func skipUpdateSpline(r *Reader) error {
	flags, err := r.U32()
	if err != nil {
		return err
	}
	switch {
	case flags&SplineFlagFinalAngle != 0:
		err = r.Skip(4)
	case flags&SplineFlagFinalTarget != 0:
		err = r.Skip(8)
	case flags&SplineFlagFinalPoint != 0:
		err = r.Skip(12)
	}
	if err != nil {
		return err
	}
	if err = r.Skip(4 + 4 + 4 + 4 + 4 + 4 + 4); err != nil {
		return err
	}
	points, err := r.U32()
	if err != nil {
		return err
	}
	if points > 1<<20 {
		return fmt.Errorf("SMSG_UPDATE_OBJECT spline has too many points: %d", points)
	}
	if err = r.Skip(int(points) * 12); err != nil {
		return err
	}
	return r.Skip(1 + 12)
}

func ParseUpdateObject(body []byte) ([]UpdateBlock, error) {
	r := NewReader(body, "SMSG_UPDATE_OBJECT")
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	if count > 1<<20 {
		return nil, fmt.Errorf("SMSG_UPDATE_OBJECT has too many blocks: %d", count)
	}
	blocks := make([]UpdateBlock, 0, count)
	for index := uint32(0); index < count; index++ {
		code, readErr := r.U8()
		if readErr != nil {
			return nil, readErr
		}
		block := UpdateBlock{Type: UpdateType(code)}
		switch block.Type {
		case UpdateValues, UpdateMovementBlock:
			if block.GUID, err = ReadPackedGUID(r); err != nil {
				return nil, err
			}
			if block.Type == UpdateValues {
				if block.Fields, err = readUpdateFields(r); err != nil {
					return nil, err
				}
			} else if block.Movement, err = readUpdateMovement(r); err != nil {
				return nil, err
			}
		case UpdateCreate, UpdateCreate2:
			if block.GUID, err = ReadPackedGUID(r); err != nil {
				return nil, err
			}
			objectType, readErr := r.U8()
			if readErr != nil {
				return nil, readErr
			}
			if objectType > uint8(ObjectTypeCorpse) {
				return nil, fmt.Errorf("SMSG_UPDATE_OBJECT has unknown object type %d", objectType)
			}
			block.ObjectType = ObjectType(objectType)
			block.Spawned = block.Type == UpdateCreate2
			if block.Movement, err = readUpdateMovement(r); err != nil {
				return nil, err
			}
			if block.Fields, err = readUpdateFields(r); err != nil {
				return nil, err
			}
		case UpdateOutOfRange, UpdateNearObjects:
			guidCount, readErr := r.U32()
			if readErr != nil {
				return nil, readErr
			}
			if guidCount > 1<<20 {
				return nil, fmt.Errorf("SMSG_UPDATE_OBJECT has too many GUIDs: %d", guidCount)
			}
			block.GUIDs = make([]uint64, guidCount)
			for guidIndex := range block.GUIDs {
				if block.GUIDs[guidIndex], err = ReadPackedGUID(r); err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("SMSG_UPDATE_OBJECT has unknown update type %d", code)
		}
		blocks = append(blocks, block)
	}
	if err = r.Finish(); err != nil {
		return nil, err
	}
	return blocks, nil
}

func ParseCompressedUpdateObject(body []byte) ([]UpdateBlock, error) {
	r := NewReader(body, "SMSG_COMPRESSED_UPDATE_OBJECT")
	expected, err := r.U32()
	if err != nil {
		return nil, err
	}
	if expected > MaxPacket {
		return nil, fmt.Errorf("SMSG_COMPRESSED_UPDATE_OBJECT is too large: %d", expected)
	}
	compressed, err := r.Take(len(body) - 4)
	if err != nil {
		return nil, err
	}
	reader, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	plain, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if uint32(len(plain)) != expected {
		return nil, fmt.Errorf("SMSG_COMPRESSED_UPDATE_OBJECT length=%d want=%d", len(plain), expected)
	}
	return ParseUpdateObject(plain)
}

func ParseDestroyObject(body []byte) (uint64, error) {
	r := NewReader(body, "SMSG_DESTROY_OBJECT")
	guid, err := r.U64()
	if err != nil {
		return 0, err
	}
	if err = r.Skip(1); err != nil {
		return 0, err
	}
	if err = r.Finish(); err != nil {
		return 0, err
	}
	return guid, nil
}
