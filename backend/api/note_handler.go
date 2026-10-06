package api

import (
	"strconv"

	"backend/model"
	"backend/service"

	"github.com/gin-gonic/gin"
)

type NoteHandler struct {
	noteService *service.NoteService
}

func NewNoteHandler(noteService *service.NoteService) *NoteHandler {
	return &NoteHandler{
		noteService: noteService,
	}
}

func (h *NoteHandler) HandleCreateNote(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	var req model.CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", err.Error())
		return
	}

	note, err := h.noteService.CreateNote(userID, &req)
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}

	SendSuccess(c, note)
}

func (h *NoteHandler) HandleListNotes(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	category := c.Query("category")
	content := c.Query("content")

	notes, total, err := h.noteService.ListNotes(userID, page, pageSize, category, content)
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}

	SendSuccess(c, gin.H{
		"items": notes,
		"total": total,
	})
}

func (h *NoteHandler) HandleUpdateNote(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	noteID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid note ID")
		return
	}

	var req model.UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", err.Error())
		return
	}

	if err := h.noteService.UpdateNote(userID, uint(noteID), &req); err != nil {
		SendError(c, "500", err.Error())
		return
	}

	SendSuccess(c, nil)
}

func (h *NoteHandler) HandleDeleteNote(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	noteID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid note ID")
		return
	}

	if err := h.noteService.DeleteNote(userID, uint(noteID)); err != nil {
		SendError(c, "500", err.Error())
		return
	}

	SendSuccess(c, nil)
}
