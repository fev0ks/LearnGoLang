package everything

import (
	"encoding/json"
	"fmt"
)

type RequestData struct {
	B NeBool   `json:"b,omitempty"`
	S string   `json:"s,omitempty"`
	F *float64 `json:"f,omitempty"`
}

type NeBool bool

type RawRequestData struct {
	B string   `json:"b,omitempty"`
	S string   `json:"s,omitempty"`
	F *float64 `json:"f,omitempty"`
}

func TryJson() {
	js1 := "{\"b\":\"1\",\"s\":\"asd\",\"f\":123.123}"
	var rd1 RequestData
	err := json.Unmarshal([]byte(js1), &rd1)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%v\n", rd1)
	js2 := "{\"b\":\"no\",\"s\":\"asd\",\"f\":123.123}"
	var rd2 RawRequestData
	err = json.Unmarshal([]byte(js2), &rd2)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%v\n", rd2)
}

func (rd RequestData) String() string {
	return fmt.Sprintf("b = %v, s = %s, f = %v", rd.B, rd.S, *rd.F)
}

func (rd RawRequestData) String() string {
	return fmt.Sprintf("b = %v, s = %s, f = %v", rd.B, rd.S, *rd.F)
}

func (b *NeBool) UnmarshalJSON(data []byte) error {
	var input string
	err := json.Unmarshal(data, &input)
	if err != nil {
		fmt.Println(err)
	}
	var boolValue NeBool
	if input == "no" || input == "false" || input == "0" {
		boolValue = false
	} else if input == "yes" || input == "true" || input == "1" {
		boolValue = true
	}
	*b = boolValue
	return nil
}

//func (rd *RequestData) UnmarshalJSON(data []byte) error {
//	var rd2 RawRequestData
//	err := json.Unmarshal(data, &rd2)
//	if err != nil {
//		fmt.Println( err)
//	}
//	var b bool
//	if rd2.B == "no" || rd2.B == "false" || rd2.B == "0" {
//		b = false
//	} else if rd2.B == "yes" || rd2.B == "true" || rd2.B == "1" {
//		b = true
//	}
//	*rd = RequestData {
//		S: rd2.S,
//		B: &b,
//		F: rd2.F,
//	}
//	return nil
//}
