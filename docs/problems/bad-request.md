# Bad Request

**Type**: `https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-request.md`
**Status**: `400 Bad Request`
**Code**: `400-01`

## Description

This error occurs when the client sends an invalid or malformed request. This could happen due to a variety of reasons, including invalid or missing request parameters, malformed JSON payloads, incorrect HTTP method usage, or unsupported content types.

Bad Request errors are typically a result of improper client behavior and should be addressed by correcting the request before retrying.

## Example JSON

```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-request.md",
  "title": "Bad Request",
  "status": 400,
  "code": "400-01",
  "detail": "The request is invalid or malformed",
  "instance": "/api/resource"
}
```
