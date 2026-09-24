package rules

import (
	"context"
	"errors"
	"testing"

	"rinha-backend-2026/internal/model"
)

func TestDecideUsesFiveNeighborsAndThreshold(t *testing.T) {
	tests := []struct {
		name       string
		fraudCount int
		approved   bool
		score      float64
	}{
		{name: "zero frauds", fraudCount: 0, approved: true, score: 0},
		{name: "two frauds", fraudCount: 2, approved: true, score: 0.4},
		{name: "threshold", fraudCount: 3, approved: false, score: 0.6},
		{name: "all frauds", fraudCount: 5, approved: false, score: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			neighbors := make([]model.Neighbor, 5)
			for index := 0; index < test.fraudCount; index++ {
				neighbors[index].Label = model.LabelFraud
			}
			decision, err := Decide(neighbors)
			if err != nil {
				t.Fatalf("decide: %v", err)
			}
			if decision.Approved != test.approved || decision.FraudScore != test.score {
				t.Errorf("decision = %#v, want approved=%v score=%v", decision, test.approved, test.score)
			}
		})
	}
}

func TestDecideRejectsWrongNeighborCount(t *testing.T) {
	if _, err := Decide(make([]model.Neighbor, 4)); err == nil {
		t.Fatal("decide with four neighbors succeeded, want error")
	}
}

func TestEnginePropagatesVectorizerAndSearcherErrors(t *testing.T) {
	vectorizerErr := errors.New("vectorizer error")
	engine := New(staticVectorizer{err: vectorizerErr}, staticSearcher{})
	if _, err := engine.Score(context.Background(), model.FraudRequest{}); !errors.Is(err, vectorizerErr) {
		t.Errorf("vectorizer error = %v, want wrapped vectorizer error", err)
	}

	searcherErr := errors.New("searcher error")
	engine = New(staticVectorizer{}, staticSearcher{err: searcherErr})
	if _, err := engine.Score(context.Background(), model.FraudRequest{}); !errors.Is(err, searcherErr) {
		t.Errorf("searcher error = %v, want wrapped searcher error", err)
	}
}

type staticVectorizer struct {
	err error
}

func (s staticVectorizer) Vectorize(model.FraudRequest) (model.Vector, error) {
	return model.Vector{}, s.err
}

type staticSearcher struct {
	err error
}

func (s staticSearcher) TopK(context.Context, model.Vector, int) ([]model.Neighbor, error) {
	if s.err != nil {
		return nil, s.err
	}
	return make([]model.Neighbor, 5), nil
}
