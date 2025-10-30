package logging

import (
    "encoding/json"
    "fmt"
)

func Log(level string, fields map[string]any) {
    if fields == nil { fields = map[string]any{} }
    fields["level"] = level
    b, _ := json.Marshal(fields)
    fmt.Println(string(b))
}

