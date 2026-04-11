package graphapi

import (
	"context"
	"fmt"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// Category is a simplified outlook category for output
type Category struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Color       string `json:"color"`
}

func (c *Client) ListCategories(ctx context.Context) ([]Category, error) {
	resp, err := c.inner.Me().Outlook().MasterCategories().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing categories: %s", graphErrorMessage(err))
	}

	var categories []Category
	for _, cat := range resp.GetValue() {
		categories = append(categories, convertCategory(cat))
	}
	return categories, nil
}

func (c *Client) CreateCategory(ctx context.Context, name, color string) (*Category, error) {
	cat := models.NewOutlookCategory()
	cat.SetDisplayName(&name)

	if color != "" {
		col, err := models.ParseCategoryColor(color)
		if err != nil {
			return nil, fmt.Errorf("invalid color %q: valid colors are preset0 through preset24, or none", color)
		}
		cat.SetColor(col.(*models.CategoryColor))
	}

	created, err := c.inner.Me().Outlook().MasterCategories().Post(ctx, cat, nil)
	if err != nil {
		return nil, fmt.Errorf("creating category: %s", graphErrorMessage(err))
	}

	result := convertCategory(created)
	return &result, nil
}

func (c *Client) DeleteCategory(ctx context.Context, categoryID string) error {
	if err := validateID(categoryID, "category ID"); err != nil {
		return err
	}
	err := c.inner.Me().Outlook().MasterCategories().ByOutlookCategoryId(categoryID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting category: %s", graphErrorMessage(err))
	}
	return nil
}

func convertCategory(cat models.OutlookCategoryable) Category {
	c := Category{}
	if cat.GetId() != nil {
		c.ID = *cat.GetId()
	}
	if cat.GetDisplayName() != nil {
		c.DisplayName = *cat.GetDisplayName()
	}
	if cat.GetColor() != nil {
		c.Color = cat.GetColor().String()
	}
	return c
}
