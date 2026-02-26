package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope matches the Python API envelope exactly.
type Envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *string     `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

type Meta struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Envelope{Success: true, Data: data})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Envelope{Success: true, Data: data})
}

func Paginated(c *gin.Context, data interface{}, total, page, limit int) {
	c.JSON(http.StatusOK, Envelope{
		Success: true,
		Data:    data,
		Meta:    &Meta{Total: total, Page: page, Limit: limit},
	})
}

func Err(c *gin.Context, status int, msg string) {
	c.JSON(status, Envelope{Success: false, Error: &msg})
}

func BadRequest(c *gin.Context, msg string) {
	Err(c, http.StatusBadRequest, msg)
}

func Unauthorized(c *gin.Context, msg string) {
	Err(c, http.StatusUnauthorized, msg)
}

func Forbidden(c *gin.Context, msg string) {
	Err(c, http.StatusForbidden, msg)
}

func NotFound(c *gin.Context, msg string) {
	Err(c, http.StatusNotFound, msg)
}

func Conflict(c *gin.Context, msg string) {
	Err(c, http.StatusConflict, msg)
}

func InternalError(c *gin.Context, msg string) {
	Err(c, http.StatusInternalServerError, msg)
}

// ErrWithData returns a structured error response with additional data payload.
func ErrWithData(c *gin.Context, status int, msg string, data interface{}) {
	c.JSON(status, gin.H{
		"success": false,
		"error":   msg,
		"data":    data,
	})
}

// UnprocessableEntity returns 422 with an error message.
func UnprocessableEntity(c *gin.Context, msg string) {
	Err(c, http.StatusUnprocessableEntity, msg)
}

// UnprocessableEntityWithData returns 422 with structured data (e.g. fix suggestions).
func UnprocessableEntityWithData(c *gin.Context, msg string, data interface{}) {
	ErrWithData(c, http.StatusUnprocessableEntity, msg, data)
}
