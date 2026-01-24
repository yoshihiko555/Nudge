package notion

import (
	"strings"

	"nudge/internal/dto"
)

type queryResponse struct {
	Results []page `json:"results"`
}

type page struct {
	ID             string                   `json:"id"`
	URL            string                   `json:"url"`
	LastEditedTime string                   `json:"last_edited_time"`
	Properties     map[string]propertyValue `json:"properties"`
	Icon           *pageIcon                `json:"icon"`
}

type propertyValue struct {
	Type     string `json:"type"`
	Title    []text `json:"title"`
	Status   *name  `json:"status"`
	Select   *name  `json:"select"`
	Checkbox *bool  `json:"checkbox"`
}

type text struct {
	PlainText string `json:"plain_text"`
}

type name struct {
	Name string `json:"name"`
}

type pageIcon struct {
	Type     string      `json:"type"`
	Emoji    string      `json:"emoji"`
	External *iconSource `json:"external"`
	File     *iconSource `json:"file"`
}

type iconSource struct {
	URL string `json:"url"`
}

type databaseResponse struct {
	DataSources []struct {
		ID string `json:"id"`
	} `json:"data_sources"`
	Properties map[string]struct {
		Type string `json:"type"`
	} `json:"properties"`
}

type blocksResponse struct {
	Results    []block `json:"results"`
	HasMore    bool    `json:"has_more"`
	NextCursor string  `json:"next_cursor"`
}

type block struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	HasChildren bool       `json:"has_children"`
	Paragraph   *blockText `json:"paragraph"`
	Heading1    *blockText `json:"heading_1"`
	Heading2    *blockText `json:"heading_2"`
	Heading3    *blockText `json:"heading_3"`
	Bulleted    *blockText `json:"bulleted_list_item"`
	Numbered    *blockText `json:"numbered_list_item"`
	ToDo        *blockText `json:"to_do"`
	Quote       *blockText `json:"quote"`
	Callout     *blockText `json:"callout"`
}

type blockText struct {
	RichText []text `json:"rich_text"`
}

func mapTasks(pages []page, titlePropertyName, statusPropertyName, checkboxPropertyName string) []dto.Task {
	out := make([]dto.Task, 0, len(pages))
	for _, p := range pages {
		title := extractTitle(p.Properties[titlePropertyName])
		status := extractStatus(p.Properties[statusPropertyName])
		checked := false
		if checkboxPropertyName != "" {
			checked = extractCheckbox(p.Properties[checkboxPropertyName])
		}
		out = append(out, dto.Task{
			ID:             p.ID,
			Title:          title,
			URL:            p.URL,
			Status:         status,
			LastEditedTime: p.LastEditedTime,
			Checked:        checked,
		})
	}
	return out
}

func extractTitle(prop propertyValue) string {
	if prop.Type != "title" {
		return ""
	}
	if len(prop.Title) == 0 {
		return ""
	}
	return prop.Title[0].PlainText
}

func extractStatus(prop propertyValue) string {
	if prop.Type == "status" && prop.Status != nil {
		return prop.Status.Name
	}
	if prop.Type == "select" && prop.Select != nil {
		return prop.Select.Name
	}
	return ""
}

func extractCheckbox(prop propertyValue) bool {
	if prop.Type != "checkbox" || prop.Checkbox == nil {
		return false
	}
	return *prop.Checkbox
}

func extractTitleFromProperties(props map[string]propertyValue) string {
	for _, prop := range props {
		if prop.Type == "title" {
			return extractTitle(prop)
		}
	}
	return ""
}

func extractTitleProperty(props map[string]propertyValue) (string, string) {
	for name, prop := range props {
		if prop.Type == "title" {
			return name, extractTitle(prop)
		}
	}
	return "", ""
}

func blocksToPlainText(blocks []block) string {
	lines := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if line := blockToLine(b); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func blockToLine(b block) string {
	text := extractBlockText(b)
	if text == "" {
		return ""
	}
	switch b.Type {
	case "heading_1":
		return "# " + text
	case "heading_2":
		return "## " + text
	case "heading_3":
		return "### " + text
	case "bulleted_list_item":
		return "- " + text
	case "numbered_list_item":
		return "1. " + text
	case "to_do":
		return "- [ ] " + text
	default:
		return text
	}
}

func extractBlockText(b block) string {
	var rich []text
	switch b.Type {
	case "paragraph":
		if b.Paragraph != nil {
			rich = b.Paragraph.RichText
		}
	case "heading_1":
		if b.Heading1 != nil {
			rich = b.Heading1.RichText
		}
	case "heading_2":
		if b.Heading2 != nil {
			rich = b.Heading2.RichText
		}
	case "heading_3":
		if b.Heading3 != nil {
			rich = b.Heading3.RichText
		}
	case "bulleted_list_item":
		if b.Bulleted != nil {
			rich = b.Bulleted.RichText
		}
	case "numbered_list_item":
		if b.Numbered != nil {
			rich = b.Numbered.RichText
		}
	case "to_do":
		if b.ToDo != nil {
			rich = b.ToDo.RichText
		}
	case "quote":
		if b.Quote != nil {
			rich = b.Quote.RichText
		}
	case "callout":
		if b.Callout != nil {
			rich = b.Callout.RichText
		}
	default:
		return ""
	}
	return joinPlainText(rich)
}

func joinPlainText(values []text) string {
	if len(values) == 0 {
		return ""
	}
	var b strings.Builder
	for _, v := range values {
		b.WriteString(v.PlainText)
	}
	return strings.TrimSpace(b.String())
}
