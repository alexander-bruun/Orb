package ingest

import (
	"reflect"
	"testing"
)

func TestSplitArtistList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"Charlie Puth", []string{"Charlie Puth"}},
		{"Charlie Puth & Coco Jones", []string{"Charlie Puth", "Coco Jones"}},
		{"Charlie Puth, Coco Jones", []string{"Charlie Puth", "Coco Jones"}},
		{"Charlie Puth; Coco Jones", []string{"Charlie Puth", "Coco Jones"}},
		{"Charlie Puth and Coco Jones", []string{"Charlie Puth", "Coco Jones"}},
		{"Charlie Puth feat. Coco Jones", []string{"Charlie Puth", "Coco Jones"}},
		{"Charlie Puth ft. Coco Jones", []string{"Charlie Puth", "Coco Jones"}},
		{"Charlie Puth featuring Coco Jones", []string{"Charlie Puth", "Coco Jones"}},
		// The problematic case
		{"Charlie Puth (feat. Coco Jones feat. Coco Jones)", []string{"Charlie Puth", "Coco Jones"}},
		{"Charlie Puth (feat. Coco Jones)", []string{"Charlie Puth", "Coco Jones"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitArtistList(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitArtistList(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFeaturedArtists(t *testing.T) {
	tests := []struct {
		input      string
		wantTitle  string
		wantNames  []string
	}{
		{"Sideways", "Sideways", nil},
		{"Sideways (feat. Coco Jones)", "Sideways", []string{"Coco Jones"}},
		{"Sideways [feat. Coco Jones]", "Sideways", []string{"Coco Jones"}},
		{"Sideways feat. Coco Jones", "Sideways", []string{"Coco Jones"}},
		{"Sideways featuring Coco Jones", "Sideways", []string{"Coco Jones"}},
		{"Sideways ft. Coco Jones", "Sideways", []string{"Coco Jones"}},
		{"Sideways (feat. Coco Jones & Kenny Loggins)", "Sideways", []string{"Coco Jones", "Kenny Loggins"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotTitle, gotNames := parseFeaturedArtists(tt.input)
			if gotTitle != tt.wantTitle {
				t.Errorf("parseFeaturedArtists(%q) title = %q, want %q", tt.input, gotTitle, tt.wantTitle)
			}
			if !reflect.DeepEqual(gotNames, tt.wantNames) {
				t.Errorf("parseFeaturedArtists(%q) names = %v, want %v", tt.input, gotNames, tt.wantNames)
			}
		})
	}
}
