// Code generated by "stringer -type=Type -linecomment"; DO NOT EDIT.

package schema

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[TypeAvro-1]
}

const _Type_name = "TypeAvro"

var _Type_index = [...]uint8{0, 8}

func (i Type) String() string {
	i -= 1
	if i < 0 || i >= Type(len(_Type_index)-1) {
		return "Type(" + strconv.FormatInt(int64(i+1), 10) + ")"
	}
	return _Type_name[_Type_index[i]:_Type_index[i+1]]
}