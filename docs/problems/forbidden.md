# Forbidden

**Type**: `https://github.com/nickbryan/httputil/blob/main/docs/problems/forbidden.md`  
**Status**: `403 Forbidden`

## Description

This error is returned when the server understands the request but refuses to fulfill it due to insufficient permissions. The client is authenticated but not authorized to perform the requested action.

A `Forbidden` error often applies to requests restricted by user role, resource access policies, or other permission-related mechanisms.

## Example JSON

```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/forbidden.md",
  "title": "Forbidden",
  "status": 403,
  "detail": "You do not have the necessary permissions to DELETE this resource",
  "instance": "/api/resource/123"
}
```
