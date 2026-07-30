package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"
)

func Write(writer io.Writer, value any, format string) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "csv":
		return writeCSV(writer, value)
	case "table":
		return writeTable(writer, value)
	default:
		return fmt.Errorf("unsupported format %q (use table, json or csv)", format)
	}
}

func writeCSV(writer io.Writer, value any) error {
	headers, rows, err := flatten(value)
	if err != nil {
		return err
	}
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write(headers); err != nil {
		return err
	}
	for _, row := range rows {
		if err := csvWriter.Write(row); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

func writeTable(writer io.Writer, value any) error {
	headers, rows, err := flatten(value)
	if err != nil {
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		for index := range row {
			row[index] = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(row[index])
		}
		if _, err := fmt.Fprintln(table, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return table.Flush()
}

func flatten(value any) ([]string, [][]string, error) {
	current := reflect.ValueOf(value)
	for current.IsValid() && current.Kind() == reflect.Pointer {
		if current.IsNil() {
			return nil, nil, fmt.Errorf("cannot render nil value")
		}
		current = current.Elem()
	}

	var items []reflect.Value
	if current.Kind() == reflect.Slice || current.Kind() == reflect.Array {
		for index := 0; index < current.Len(); index++ {
			item := current.Index(index)
			for item.Kind() == reflect.Pointer {
				item = item.Elem()
			}
			items = append(items, item)
		}
		if len(items) == 0 {
			return []string{"result"}, nil, nil
		}
	} else {
		items = []reflect.Value{current}
	}
	if items[0].Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("table and csv output require a struct or slice of structs")
	}

	itemType := items[0].Type()
	headers := make([]string, 0, itemType.NumField())
	fieldIndexes := make([]int, 0, itemType.NumField())
	for index := 0; index < itemType.NumField(); index++ {
		field := itemType.Field(index)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" {
			name = field.Name
		}
		if name == "-" {
			continue
		}
		headers = append(headers, name)
		fieldIndexes = append(fieldIndexes, index)
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		row := make([]string, 0, len(fieldIndexes))
		for _, index := range fieldIndexes {
			row = append(row, fmt.Sprint(item.Field(index).Interface()))
		}
		rows = append(rows, row)
	}
	return headers, rows, nil
}
