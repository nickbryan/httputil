# Server Error
**Type**: `https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md`  
**Status**: `500 Internal Server Error`
**Code**: `500-01`

## Description
This error is returned when the server encountered an unexpected condition that prevented it from fulfilling the 
request. It is a generic error message for failures that do not fit into other error categories.

`Server Error` usually signals a bug, unhandled exception, or an infrastructure issue, and it indicates that the 
problem is on the server, not with the client.

## Example JSON
```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md",
  "title": "Server Error",
  "status": 500,
  "code": "500-01",
  "detail": "The server encountered an unexpected internal error",
  "instance": "/api/resource"
}
```
