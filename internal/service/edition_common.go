package service

import "github.com/h-wb/sento/internal/store"

func editionByID(cols []store.Edition, id int64) *store.Edition {
	for i := range cols {
		if cols[i].ID == id {
			return &cols[i]
		}
	}
	return nil
}
