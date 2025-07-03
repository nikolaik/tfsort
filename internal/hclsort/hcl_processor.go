package hclsort

import (
	"fmt"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// ParseHCLContent parses the HCL source byte slice using hclwrite.
func ParseHCLContent(
	src []byte,
	filename string,
) (*hclwrite.File, error) {
	file, diags := hclwrite.ParseConfig(
		src,
		filename,
		hcl.Pos{Line: 1, Column: 1},
	)
	if diags.HasErrors() {
		return nil, fmt.Errorf(
			"error parsing HCL content from '%s': %w",
			filename,
			diags,
		)
	}
	return file, nil
}

// sortRequiredProvidersInBlock sorts the entries in any required_providers block.
func sortRequiredProvidersInBlock(block *hclwrite.Block) {
	for _, b := range block.Body().Blocks() {
		if b.Type() != "required_providers" {
			continue
		}
		body := b.Body()
		attrs := body.Attributes()

		providerNames := make([]string, 0, len(attrs))
		for name := range attrs {
			providerNames = append(providerNames, name)
		}
		sort.Strings(providerNames)

		body.Clear()
		body.AppendNewline()

		for i, name := range providerNames {
			attr := attrs[name]
			tokens := attr.BuildTokens(nil)

			start, end := 0, len(tokens)
			for start < end && tokens[start].Type == hclsyntax.TokenNewline {
				start++
			}
			for end > start && tokens[end-1].Type == hclsyntax.TokenNewline {
				end--
			}
			body.AppendUnstructuredTokens(tokens[start:end])
			if i+1 < len(providerNames) {
				body.AppendNewline()
			}
		}
		body.AppendNewline()
	}
}

// sortLocalsBlock sorts the top‐level assignments in a locals block.
func sortLocalsBlock(block *hclwrite.Block) {
	body := block.Body()
	attrs := body.Attributes()

	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)

	body.Clear()
	body.AppendNewline()
	for i, name := range names {
		attr := attrs[name]
		tokens := attr.BuildTokens(nil)

		start, end := 0, len(tokens)
		for start < end && tokens[start].Type == hclsyntax.TokenNewline {
			start++
		}
		for end > start && tokens[end-1].Type == hclsyntax.TokenNewline {
			end--
		}

		body.AppendUnstructuredTokens(tokens[start:end])
		if i+1 < len(names) {
			body.AppendNewline()
		}
	}
	body.AppendNewline()
}

// sortResourceParams sorts the resource parameters for a resource/data block according to the Terraform Style Guide
func sortResourceParams(block *hclwrite.Block) {
	body := block.Body()
	attrs := body.Attributes()
	blocks := body.Blocks()

	// First in block
	const metaArgCount = "count"
	const metaArgForEach = "for_each"
	// Last in block
	const metaBlockLifecycle = "lifecycle"
	const metaArgDependsOn = "depends_on"
	namesFirst := []string{}
	hasDependsOn := false
	// blocksLast := []*hclwrite.Block{}
	names := []string{}
	for name := range attrs {
		switch name {
		case metaArgCount, metaArgForEach:
			namesFirst = append(namesFirst, name)
		case metaArgDependsOn:
			hasDependsOn = true
		default:
			names = append(names, name)
		}
	}
	sort.Strings(names)

	// Rebuild body
	body.Clear()
	body.AppendNewline()

	// Add `for_each` or `count` attribute
	for _, name := range namesFirst {
		attr := attrs[name]
		tokens := attr.BuildTokens(nil)

		// Remove leading and trailing newlines from tokens
		start, end := 0, len(tokens)
		for start < end && tokens[start].Type == hclsyntax.TokenNewline {
			start++
		}
		for end > start && tokens[end-1].Type == hclsyntax.TokenNewline {
			end--
		}
		body.AppendUnstructuredTokens(tokens[start:end])
		body.AppendNewline()
	}

	if len(namesFirst) > 0 {
		body.AppendNewline()
	}

	// Add attributes
	for idx, name := range names {
		attr := attrs[name]
		tokens := attr.BuildTokens(nil)

		// Remove leading and trailing newlines from tokens
		start, end := 0, len(tokens)
		for start < end && tokens[start].Type == hclsyntax.TokenNewline {
			start++
		}
		for end > start && tokens[end-1].Type == hclsyntax.TokenNewline {
			end--
		}

		body.AppendUnstructuredTokens(tokens[start:end])
		// Append a newline after each attribute except the last one
		if idx+1 < len(names) {
			body.AppendNewline()
		}
	}

	if len(blocks) > 0 && (len(names) > 0 || len(namesFirst) > 0) {
		body.AppendNewline()
		body.AppendNewline()
	}

	// Add the blocks
	for idx, block := range blocks {
		body.AppendBlock(block)

		if idx+1 < len(blocks) {
			body.AppendNewline()
		}
	}
	// FIXME: Special care for lifecycle block

	// Add depends_on attribute
	if hasDependsOn {
		if len(blocks) > 0 || len(names) > 0 || len(namesFirst) > 0 {
			body.AppendNewline()
		}

		attr := attrs["depends_on"]
		tokens := attr.BuildTokens(nil)

		// Remove leading and trailing newlines from tokens
		start, end := 0, len(tokens)
		for start < end && tokens[start].Type == hclsyntax.TokenNewline {
			start++
		}
		for end > start && tokens[end-1].Type == hclsyntax.TokenNewline {
			end--
		}
		body.AppendUnstructuredTokens(tokens[start:end])
	}

	body.AppendNewline()
}

// ProcessAndSortBlocks extracts sortable blocks (variables, outputs, locals, terraform) and sorts them.
func ProcessAndSortBlocks(
	file *hclwrite.File,
	allowedBlocks map[string]bool,
) *hclwrite.File {
	for _, block := range file.Body().Blocks() {
		switch block.Type() {
		case "terraform":
			sortRequiredProvidersInBlock(block)
		case "resource":
			sortResourceParams(block)
		case "locals":
			sortLocalsBlock(block)
		}
	}

	body := file.Body()
	originalBlocks := body.Blocks()

	sortableItems := make([]*SortableBlock, 0)
	otherBlocks := make([]*hclwrite.Block, 0)

	for _, block := range originalBlocks {
		blockType := block.Type()
		if allowedBlocks[blockType] && len(block.Labels()) > 0 {
			sortableItems = append(sortableItems, &SortableBlock{
				Name:  block.Labels()[0],
				Block: block,
			})
		} else {
			otherBlocks = append(otherBlocks, block)
		}
	}

	sort.Slice(sortableItems, func(i, j int) bool {
		return sortableItems[i].Name < sortableItems[j].Name
	})

	body.Clear()

	for i, block := range otherBlocks {
		body.AppendBlock(block)
		if i < len(otherBlocks)-1 || len(sortableItems) > 0 {
			body.AppendNewline()
		}
	}

	for i, sb := range sortableItems {
		body.AppendBlock(sb.Block)
		if i < len(sortableItems)-1 {
			body.AppendNewline()
		}
	}

	return file
}

// FormatHCLBytes formats the HCL file's content into a byte slice.
func FormatHCLBytes(file *hclwrite.File) []byte {
	return hclwrite.Format(file.Bytes())
}
