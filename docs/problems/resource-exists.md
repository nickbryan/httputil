# Resource Exists

**Type**: `https://github.com/nickbryan/httputil/blob/main/docs/problems/resource-exists.md`  
**Status**: `409 Conflict`

## Description

This error occurs when a client tries to create a resource that already exists. For example, attempting to create a user with an email that is already registered.

The `Resource Exists` error indicates a conflict with the current state of the server and the action the client tried to perform.

## Example JSON

```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/resource-exists.md",
  "title": "Resource Exists",
  "status": 409,
  "detail": "A resource already exists with the specified identifier",
  "instance": "/api/resource"
}
```
