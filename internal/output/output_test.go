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

func TestCSVNeutralizesSpreadsheetFormulas(t *testing.T) {
	var output bytes.Buffer
	rows := []row{
		{Name: "=HYPERLINK(\"https://example.invalid\")", Count: 1},
		{Name: "+cmd", Count: 2},
		{Name: "-1+2", Count: 3},
		{Name: "@SUM(1,2)", Count: 4},
	}
	if err := Write(&output, rows, "csv"); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`'=HYPERLINK(""https://example.invalid"")`,
		`'+cmd`,
		`'-1+2`,
		`'@SUM(1,2)`,
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("CSV output %q lacks neutralized %q", output.String(), expected)
		}
	}
}
