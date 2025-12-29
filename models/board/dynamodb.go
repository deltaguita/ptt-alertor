package board

import (
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
)

// DynamoDB - disabled, using Redis only
type DynamoDB struct{}

func (DynamoDB) GetArticles(boardName string) (articles article.Articles) {
	return
}

func (DynamoDB) Save(boardName string, articles article.Articles) error {
	return nil
}

func (DynamoDB) Delete(boardName string) error {
	return nil
}
