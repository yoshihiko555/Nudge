package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"nudge/internal/dto"
)

func (c *Client) FetchTemplate(ctx context.Context, pageID, notionVersion string) (dto.BrainTemplate, error) {
	if strings.TrimSpace(pageID) == "" {
		return dto.BrainTemplate{}, fmt.Errorf("template_page_id is required")
	}
	var p page
	path := fmt.Sprintf("/v1/pages/%s", pageID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &p, notionVersion); err != nil {
		return dto.BrainTemplate{}, err
	}
	blocks, err := c.listAllBlockChildren(ctx, pageID, notionVersion)
	if err != nil {
		return dto.BrainTemplate{}, err
	}
	return dto.BrainTemplate{
		Title: extractTitleFromProperties(p.Properties),
		Body:  blocksToPlainText(blocks),
	}, nil
}

func (c *Client) CreatePageFromTemplate(ctx context.Context, databaseID, templatePageID, body, notionVersion string) (dto.CreatedPage, error) {
	if strings.TrimSpace(databaseID) == "" {
		return dto.CreatedPage{}, fmt.Errorf("database_id is required")
	}
	if strings.TrimSpace(templatePageID) == "" {
		return dto.CreatedPage{}, fmt.Errorf("template_page_id is required")
	}
	var tpl page
	path := fmt.Sprintf("/v1/pages/%s", templatePageID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &tpl, notionVersion); err != nil {
		return dto.CreatedPage{}, err
	}
	titlePropertyName, title := extractTitleProperty(tpl.Properties)
	if strings.TrimSpace(titlePropertyName) == "" {
		return dto.CreatedPage{}, fmt.Errorf("title_property_name is required")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "無題"
	}
	properties := map[string]any{
		titlePropertyName: map[string]any{
			"title": []map[string]any{{
				"text": map[string]any{"content": title},
			}},
		},
	}
	payload := map[string]any{
		"parent":     map[string]any{"database_id": databaseID},
		"properties": properties,
	}
	if icon := buildPageIcon(tpl.Icon); icon != nil {
		payload["icon"] = icon
	}
	body = strings.TrimRight(body, "\n")
	if strings.TrimSpace(body) != "" {
		payload["children"] = []map[string]any{
			{
				"object": "block",
				"type":   "paragraph",
				"paragraph": map[string]any{
					"rich_text": []map[string]any{{
						"type": "text",
						"text": map[string]any{"content": body},
					}},
				},
			},
		}
	}
	var resp page
	if err := c.doJSON(ctx, http.MethodPost, "/v1/pages", payload, &resp, notionVersion); err != nil {
		return dto.CreatedPage{}, err
	}
	return dto.CreatedPage{ID: resp.ID, URL: resp.URL}, nil
}

func (c *Client) listAllBlockChildren(ctx context.Context, blockID, notionVersion string) ([]block, error) {
	if strings.TrimSpace(blockID) == "" {
		return nil, fmt.Errorf("block_id is required")
	}
	var out []block
	cursor := ""
	for {
		path := fmt.Sprintf("/v1/blocks/%s/children?page_size=100", blockID)
		if cursor != "" {
			path = path + "&start_cursor=" + url.QueryEscape(cursor)
		}
		var resp blocksResponse
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp, notionVersion); err != nil {
			return nil, err
		}
		out = append(out, resp.Results...)
		if !resp.HasMore || resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	return out, nil
}

func buildPageIcon(icon *pageIcon) map[string]any {
	if icon == nil {
		return nil
	}
	switch icon.Type {
	case "emoji":
		if strings.TrimSpace(icon.Emoji) == "" {
			return nil
		}
		return map[string]any{
			"type":  "emoji",
			"emoji": icon.Emoji,
		}
	case "external":
		if icon.External == nil || strings.TrimSpace(icon.External.URL) == "" {
			return nil
		}
		return map[string]any{
			"type": "external",
			"external": map[string]any{
				"url": icon.External.URL,
			},
		}
	case "file":
		if icon.File == nil || strings.TrimSpace(icon.File.URL) == "" {
			return nil
		}
		return map[string]any{
			"type": "file",
			"file": map[string]any{
				"url": icon.File.URL,
			},
		}
	default:
		return nil
	}
}
