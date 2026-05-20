package profilegen

import (
	"fmt"
	"strconv"
	"strings"
)

func parseSingleSelection(input string, count int) (int, error) {
	indexes, err := parseMultiSelection(input, count)
	if err != nil {
		return 0, err
	}
	if len(indexes) != 1 {
		return 0, fmt.Errorf("select exactly one number")
	}
	return indexes[0], nil
}

func parseMultiSelection(input string, count int) ([]int, error) {
	if count < 1 {
		return nil, fmt.Errorf("no fields available")
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("selection is required")
	}

	parts := strings.Split(input, ",")
	indexes := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))

	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			return nil, fmt.Errorf("selection contains an empty item")
		}

		number, err := strconv.Atoi(token)
		if err != nil {
			return nil, fmt.Errorf("selection %q is not a number", token)
		}
		if number < 1 || number > count {
			return nil, fmt.Errorf("selection %d is out of range 1-%d", number, count)
		}

		index := number - 1
		if _, exists := seen[index]; exists {
			return nil, fmt.Errorf("selection %d is duplicated", number)
		}
		seen[index] = struct{}{}
		indexes = append(indexes, index)
	}

	return indexes, nil
}
