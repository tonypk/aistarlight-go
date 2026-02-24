package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSON is a raw JSON type for JSONB columns.
type JSON json.RawMessage

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return []byte(j), nil
}

func (j *JSON) Scan(src interface{}) error {
	if src == nil {
		*j = JSON("{}")
		return nil
	}
	switch v := src.(type) {
	case []byte:
		*j = JSON(v)
	case string:
		*j = JSON(v)
	default:
		return fmt.Errorf("unsupported type for JSON: %T", src)
	}
	return nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return []byte(j), nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	if data == nil {
		return nil
	}
	*j = JSON(data)
	return nil
}
