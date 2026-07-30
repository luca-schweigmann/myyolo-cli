package output

import (
	"bytes"
	"strings"
	"testing"
)

type row struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestCSVAndTable(t *testing.T) {
	for _, format := range []string{"csv", "table"} {
		var output bytes.Buffer
		if err := Write(&output, []row{{Name: "A", Count: 2}}, format); err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{"name", "count", "A", "2"} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("%s output %q lacks %q", format, output.String(), expected)
			}
		}
	}
}
