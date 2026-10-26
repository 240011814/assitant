package service

import (
	"errors"

	"backend/model"

	"gorm.io/gorm"
)

type NoteService struct{}

func NewNoteService() *NoteService {
	return &NoteService{}
}

func (s *NoteService) CreateNote(userID uint, req *model.CreateNoteRequest) (*model.Note, error) {
	note := &model.Note{
		UserID:   userID,
		Title:    req.Title,
		Category: req.Category,
		Content:  req.Content,
	}

	if err := DB.Create(note).Error; err != nil {
		return nil, err
	}

	return note, nil
}

func (s *NoteService) ListNotes(userID uint, page, pageSize int, category, content string) ([]model.Note, int64, error) {
	var notes []model.Note
	var total int64

	query := DB.Model(&model.Note{}).Where("user_id = ?", userID)

	if category != "" {
		query = query.Where("category LIKE ?", "%"+category+"%")
	}
	if content != "" {
		query = query.Where("content LIKE ? OR title LIKE ?", "%"+content+"%", "%"+content+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&notes).Error; err != nil {
		return nil, 0, err
	}

	return notes, total, nil
}

func (s *NoteService) UpdateNote(userID uint, noteID uint, req *model.UpdateNoteRequest) error {
	var note model.Note
	if err := DB.Where("id = ? AND user_id = ?", noteID, userID).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("note not found or no permission")
		}
		return err
	}

	updates := map[string]interface{}{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Category != "" {
		updates["category"] = req.Category
	}
	if req.Content != "" {
		updates["content"] = req.Content
	}

	return DB.Model(&note).Updates(updates).Error
}

func (s *NoteService) DeleteNote(userID uint, noteID uint) error {
	result := DB.Where("id = ? AND user_id = ?", noteID, userID).Delete(&model.Note{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("note not found or no permission")
	}
	return nil
}
