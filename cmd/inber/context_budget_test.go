package main

import (
	"testing"
)

func TestContextBudget_FirstTurn(t *testing.T) {
	e := &Engine{TurnCounter: 0}
	minImp, budget := e.contextBudget("hello")

	if minImp != 0 {
		t.Errorf("first turn minImportance = %f, want 0", minImp)
	}
	if budget != 4000 {
		t.Errorf("first turn budget = %d, want 4000", budget)
	}
}

func TestContextBudget_NormalTurn(t *testing.T) {
	e := &Engine{TurnCounter: 3}
	minImp, budget := e.contextBudget("fix the build error")

	if minImp != 0 {
		t.Errorf("normal turn minImportance = %f, want 0", minImp)
	}
	if budget != 6000 {
		t.Errorf("normal turn budget = %d, want 6000", budget)
	}
}

func TestContextBudget_LongMessage(t *testing.T) {
	e := &Engine{TurnCounter: 2}
	// >300 tokens (~1200 chars)
	longMsg := ""
	for i := 0; i < 400; i++ {
		longMsg += "word "
	}
	_, budget := e.contextBudget(longMsg)

	if budget < 10000 {
		t.Errorf("long message budget = %d, want >= 10000", budget)
	}
}

func TestContextBudget_VeryLongMessage(t *testing.T) {
	e := &Engine{TurnCounter: 2}
	// >1000 tokens (~4000 chars)
	veryLong := ""
	for i := 0; i < 1200; i++ {
		veryLong += "word "
	}
	_, budget := e.contextBudget(veryLong)

	if budget < 15000 {
		t.Errorf("very long message budget = %d, want >= 15000", budget)
	}
}

func TestContextBudget_SingleError(t *testing.T) {
	e := &Engine{TurnCounter: 5, consecutiveErrors: 1}
	_, budget := e.contextBudget("fix it")

	if budget != 20000 {
		t.Errorf("single error budget = %d, want 20000", budget)
	}
}

func TestContextBudget_LastTurnError(t *testing.T) {
	e := &Engine{TurnCounter: 5, lastTurnHadError: true}
	_, budget := e.contextBudget("try again")

	if budget != 20000 {
		t.Errorf("lastTurnHadError budget = %d, want 20000", budget)
	}
}

func TestContextBudget_ThreeErrors(t *testing.T) {
	e := &Engine{TurnCounter: 8, consecutiveErrors: 3}
	_, budget := e.contextBudget("still broken")

	if budget != 35000 {
		t.Errorf("3 errors budget = %d, want 35000", budget)
	}
}

func TestContextBudget_FiveErrors(t *testing.T) {
	e := &Engine{TurnCounter: 10, consecutiveErrors: 5}
	_, budget := e.contextBudget("completely stuck")

	if budget != 50000 {
		t.Errorf("5 errors budget = %d, want 50000", budget)
	}
}

func TestContextBudget_LongSession(t *testing.T) {
	e := &Engine{TurnCounter: 20}
	_, budget := e.contextBudget("short msg")

	if budget != 8000 {
		t.Errorf("long session budget = %d, want 8000", budget)
	}
}

func TestContextBudget_MinImportanceAlwaysZero(t *testing.T) {
	cases := []struct {
		name string
		e    Engine
	}{
		{"first turn", Engine{TurnCounter: 0}},
		{"normal", Engine{TurnCounter: 3}},
		{"error", Engine{TurnCounter: 5, consecutiveErrors: 2}},
		{"stuck", Engine{TurnCounter: 10, consecutiveErrors: 5}},
		{"long session", Engine{TurnCounter: 20}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			minImp, _ := tc.e.contextBudget("test")
			if minImp != 0 {
				t.Errorf("minImportance = %f, want 0 for all cases", minImp)
			}
		})
	}
}

func TestContextBudget_Escalation(t *testing.T) {
	// Budget should strictly increase as errors increase
	budgets := make([]int, 6)
	for errors := 0; errors < 6; errors++ {
		e := &Engine{TurnCounter: 5, consecutiveErrors: errors}
		_, budgets[errors] = e.contextBudget("test")
	}

	for i := 1; i < len(budgets); i++ {
		if budgets[i] < budgets[i-1] {
			t.Errorf("budget should increase with errors: errors=%d budget=%d < errors=%d budget=%d",
				i, budgets[i], i-1, budgets[i-1])
		}
	}
}
