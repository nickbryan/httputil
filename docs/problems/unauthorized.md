# Unauthorized

**Type**: `https://github.com/nickbryan/httputil/blob/main/docs/problems/unauthorized.md`  
**Status**: `401 Unauthorized`
**Code**: `401-01`

## Description

This error is returned when the client must authenticate itself to access a resource. Either the `Authorization` header is missing or invalid credentials were provided.

The `Unauthorized` error indicates that the resource is protected and requires authentication.

## Example JSON

```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/unauthorized.md",
  "title": "Unauthorized",
  "status": 401,
  "code": "401-01",
  "detail": "You must be authenticated to GET this resource",
  "instance": "/api/resource"
}
```
