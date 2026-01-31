package a

// SmallStruct is a small struct (32 bytes on 64-bit)
type SmallStruct struct {
	ID       int64
	Name     string
	Age      int32
	IsActive bool
}

// LargeStruct is larger than the default threshold (1024 bytes)
type LargeStruct struct {
	Field1 [512]byte
	Field2 [512]byte
	Field3 [512]byte
}

// --- Return type checks ---

func GetSmallStruct() *SmallStruct { // want "consider returning value instead of pointer: SmallStruct is .* bytes"
	return &SmallStruct{}
}

// OK: returns nil
func FindSmallStruct(id int) *SmallStruct {
	if id == 0 {
		return nil
	}
	return &SmallStruct{ID: int64(id)}
}

// OK: struct is large
func GetLargeStruct() *LargeStruct {
	return &LargeStruct{}
}

// --- Slice checks ---

func GetSmallStructs() []*SmallStruct { // want "consider using \\[\\]SmallStruct instead of \\[\\]\\*SmallStruct"
	return []*SmallStruct{}
}

// OK: struct is large
func GetLargeStructs() []*LargeStruct {
	return []*LargeStruct{}
}

// --- Method receiver checks ---

func (s *SmallStruct) FullName() string { // want "consider using value receiver: SmallStruct is .* bytes .* method doesn't mutate receiver"
	return s.Name
}

// OK: mutates receiver
func (s *SmallStruct) SetName(name string) {
	s.Name = name
}

// OK: mutates receiver field
func (s *SmallStruct) IncrementAge() {
	s.Age++
}

// OK: struct is large
func (l *LargeStruct) GetField1() []byte {
	return l.Field1[:]
}

// --- Variable declaration checks ---

func variableDeclarations() {
	var items []*SmallStruct // want "consider using \\[\\]a.SmallStruct instead of \\[\\]\\*a.SmallStruct"
	_ = items

	// OK: struct is large
	var largeItems []*LargeStruct
	_ = largeItems
}

func makeSlice() {
	items := make([]*SmallStruct, 10) // want "consider using \\[\\]a.SmallStruct instead of \\[\\]\\*a.SmallStruct"
	_ = items

	// OK: struct is large
	largeItems := make([]*LargeStruct, 10)
	_ = largeItems
}

func nilUsageInSlice() {
	// OK: uses nil comparison
	items := make([]*SmallStruct, 10)
	if items[0] == nil {
		return
	}

	// OK: assigns nil
	items2 := make([]*SmallStruct, 10)
	items2[0] = nil
	_ = items2
}

// --- Nolint checks ---

//nolint:pointless
func GetSmallStructNolint() *SmallStruct {
	return &SmallStruct{}
}

//pointless:ignore
func GetSmallStructIgnore() *SmallStruct {
	return &SmallStruct{}
}

// nolint (blanket)
func GetSmallStructBlanket() *SmallStruct {
	return &SmallStruct{}
}
