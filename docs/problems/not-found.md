# Not Found
**Type**: `https://github.com/nickbryan/httputil/blob/main/docs/problems/not-found.md`  
**Status**: `404 Not Found`
**Code**: `404-01`

## Description
This error is returned when the requested resource does not exist or cannot be found on the server. It may
also be returned when a specific API endpoint is not implemented or the provided URL is invalid.

The `Not Found` error indicates that the server could not find a match for the provided URL or identifier.

## Example JSON
```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/not-found.md",
  "title": "Not Found",
  "status": 404,
  "code": "404-01",
  "detail": "The requested resource was not found",
  "instance": "/api/resource/99999"
}
```
