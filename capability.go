package llmspecs

import (
	"fmt"
	"strings"
)

// Capability represents a model's features or modalities using bitmasks.
type Capability uint64

const (
	// Modalities (0-15 bit)
	ModalityTextIn Capability = 1 << iota
	ModalityTextOut
	ModalityImageIn
	ModalityImageOut
	ModalityAudioIn
	ModalityAudioOut
	ModalityVideoIn
	ModalityVideoOut
	ModalityFileIn
	ModalityFileOut
)

const (
	// Features (16-31 bit)
	CapFunctionCall Capability = 1 << (16 + iota)
	CapJsonMode
	CapSystemPrompt
)

// 建立一个内部映射表，用于快速匹配字符串
// 建议按位序排列，这样生成的字符串切片也是有序的
var capabilityNames = []struct {
	mask Capability
	name string
}{
	{ModalityTextIn, "TextIn"},
	{ModalityTextOut, "TextOut"},
	{ModalityImageIn, "ImageIn"},
	{ModalityImageOut, "ImageOut"},
	{ModalityAudioIn, "AudioIn"},
	{ModalityAudioOut, "AudioOut"},
	{ModalityVideoIn, "VideoIn"},
	{ModalityVideoOut, "VideoOut"},
	{ModalityFileIn, "FileIn"},
	{ModalityFileOut, "FileOut"},
	{CapFunctionCall, "FunctionCall"},
	{CapJsonMode, "JsonMode"},
	{CapSystemPrompt, "SystemPrompt"},
}

// Has checks if the capability set contains the given capability.
func (c Capability) Has(other Capability) bool {
	return c&other != 0
}

// ToStrings 将当前的位掩码组合分解为字符串切片
func (c Capability) ToStrings() []string {
	if c == 0 {
		return []string{}
	}

	var names []string
	for _, entry := range capabilityNames {
		if c.Has(entry.mask) {
			names = append(names, entry.name)
		}
	}

	// 研发细节：处理可能存在的未定义位（版本兼容性考虑）
	// 如果有些位在 capabilityNames 里没定义，但 c 里面有，可以记录为 Unknown
	definedMask := Capability(0)
	for _, entry := range capabilityNames {
		definedMask |= entry.mask
	}
	if c & ^definedMask != 0 {
		names = append(names, fmt.Sprintf("Unknown(0x%X)", uint64(c & ^definedMask)))
	}

	return names
}

// String 实现 fmt.Stringer 接口
// 这样在 fmt.Printf("%v", cap) 时会自动调用
func (c Capability) String() string {
	names := c.ToStrings()
	if len(names) == 0 {
		return "None"
	}
	return strings.Join(names, "|")
}
