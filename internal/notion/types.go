package notion

import "nudge/internal/dto"

type queryResponse struct {
	Results []page `json:"results"`
}

type page struct {
	ID             string                   `json:"id"`
	URL            string                   `json:"url"`
	LastEditedTime string                   `json:"last_edited_time"`
	Properties     map[string]propertyValue `json:"properties"`
}

type propertyValue struct {
	Type   string `json:"type"`
	Title  []text `json:"title"`
	Status *name  `json:"status"`
	Select *name  `json:"select"`
}

type text struct {
	PlainText string `json:"plain_text"`
}

type name struct {
	Name string `json:"name"`
}

type databaseResponse struct {
	DataSources []struct {
		ID string `json:"id"`
	} `json:"data_sources"`
	Properties map[string]struct {
		Type string `json:"type"`
	} `json:"properties"`
}

func mapTasks(pages []page, titlePropertyName, statusPropertyName string) []dto.Task {
	out := make([]dto.Task, 0, len(pages))
	for _, p := range pages {
		title := extractTitle(p.Properties[titlePropertyName])
		status := extractStatus(p.Properties[statusPropertyName])
		out = append(out, dto.Task{
			ID:             p.ID,
			Title:          title,
			URL:            p.URL,
			Status:         status,
			LastEditedTime: p.LastEditedTime,
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
