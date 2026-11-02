package world

import "encoding/binary"

// Vehicle packets the client sends.
const (
	ClientDismissControlledVehicle ClientOpcode = 0x046D
	ClientRequestVehicleExit       ClientOpcode = 0x0476
	ClientRequestVehiclePrevSeat   ClientOpcode = 0x0477
	ClientRequestVehicleNextSeat   ClientOpcode = 0x0478
	ClientRequestVehicleSwitchSeat ClientOpcode = 0x0479
	ClientChangeSeatsOnVehicle     ClientOpcode = 0x049B
	ClientControllerEjectPassenger ClientOpcode = 0x04A9
)

// SetProficiencyPacket is SMSG_SET_PROFICIENCY: the arms or armour of an item
// class the character may use.
const SetProficiencyPacket uint16 = 0x0127

// ClientSocketGems is CMSG_SOCKET_GEMS.
const ClientSocketGems ClientOpcode = 0x0347

// BuildSocketGems is the body of CMSG_SOCKET_GEMS: the item and the gem put in
// each of its three sockets (zero for none).
func BuildSocketGems(item uint64, gems [3]uint64) []byte {
	body := binary.LittleEndian.AppendUint64(nil, item)
	for _, gem := range gems {
		body = binary.LittleEndian.AppendUint64(body, gem)
	}
	return body
}
