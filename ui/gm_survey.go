package ui

import (
	"encoding/binary"

	lua "github.com/yuin/gopher-lua"
)

// The survey a game master's answer to a ticket can come with. The questions
// and their possible answers are read from the client's tables.
const (
	gmSurveyOpcode    = 0x032A
	gmSurveyQuestions = 10
)

// SurveyTables are the survey tables of the client.
type SurveyTables struct {
	// Current is the survey shown for a language.
	Current map[uint32]uint32
	// Surveys are the question ids of each survey.
	Surveys map[uint32][gmSurveyQuestions]uint32
	// Questions are the question texts by id.
	Questions map[uint32]string
	// Answers are the possible answers of each question id by position.
	Answers map[uint32][]SurveyAnswer
}

type SurveyAnswer struct {
	Sort uint32
	Text string
}

type gmSurveyState struct {
	tables   SurveyTables
	language uint32
	ranks    [gmSurveyQuestions]uint8
	comments [gmSurveyQuestions]string
	comment  string
}

// SetSurveyTables installs the survey tables of the client.
func (rt *Runtime) SetSurveyTables(tables SurveyTables) { rt.gmSurvey.tables = tables }

func (state *gmSurveyState) surveyID() uint32 { return state.tables.Current[state.language] }

// question is the id of the nth question of the current survey (from 1).
func (state *gmSurveyState) question(index int) uint32 {
	if index < 1 || index > gmSurveyQuestions {
		return 0
	}
	return state.tables.Surveys[state.surveyID()][index-1]
}

func registerGMSurveyAPI(rt *Runtime) {
	L := rt.L
	state := &rt.gmSurvey
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GMSurveyQuestion", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GMSurveyGetQuestion(index)")
		}
		if id := state.question(L.CheckInt(1)); id != 0 {
			if text, ok := state.tables.Questions[id]; ok {
				L.Push(lua.LString(text))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("GMSurveyAnswer", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GMSurveyGetAnswer(questionIndex, answerIndex)")
		}
		if id := state.question(L.CheckInt(1)); id != 0 {
			if _, ok := state.tables.Questions[id]; ok {
				for _, answer := range state.tables.Answers[id] {
					if int(answer.Sort) == L.CheckInt(2)-1 {
						L.Push(lua.LString(answer.Text))
						return 1
					}
				}
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("GMSurveyAnswerSubmit", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber || L.Get(3).Type() != lua.LTString {
			L.RaiseError("Usage: GMSurveyAnswerSubmit(question, rank, comment)")
		}
		index := L.CheckInt(1)
		if index < 1 || index > gmSurveyQuestions {
			L.RaiseError("GMSurveyAnswerSubmit: Questions limited from %d to %d", 1, gmSurveyQuestions)
		}
		state.ranks[index-1] = uint8(L.CheckInt(2))
		state.comments[index-1] = L.CheckString(3)
		return 0
	})
	reg("GMSurveyCommentSubmit", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: GMSurveyCommentSubmit(comment)")
		}
		state.comment = L.CheckString(1)
		return 0
	})
	reg("GMSurveySubmit", func(L *lua.LState) int {
		body := binary.LittleEndian.AppendUint32(nil, state.surveyID())
		for index := 1; index <= gmSurveyQuestions; index++ {
			question := state.question(index)
			body = binary.LittleEndian.AppendUint32(body, question)
			if question == 0 {
				break
			}
			body = append(body, state.ranks[index-1])
			body = append(append(body, state.comments[index-1]...), 0)
		}
		body = append(append(body, state.comment...), 0)
		rt.send(gmSurveyOpcode, body)
		state.ranks, state.comments, state.comment = [gmSurveyQuestions]uint8{}, [gmSurveyQuestions]string{}, ""
		return 0
	})
}
