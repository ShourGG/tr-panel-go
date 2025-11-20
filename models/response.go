package models
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}
func SuccessResponse(data interface{}) Response {
	return Response{
		Success: true,
		Data:    data,
	}
}
func ErrorResponse(err string) Response {
	return Response{
		Success: false,
		Error:   err,
	}
}
func MessageResponse(msg string) Response {
	return Response{
		Success: true,
		Message: msg,
	}
}
