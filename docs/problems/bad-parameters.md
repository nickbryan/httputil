# Bad Parameters
**Type**: `https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-parameters.md`
**Status**: `400 Bad Request`
**Code**: `400-02`

## Description
This error occurs when the client sends a request with invalid or malformed parameters. This could happen due to a
variety of reasons, including invalid or missing parameters in the request headers, path, or query string. These issues
are specifically detected and reported by the `problem.BadParameters` function.

Bad Parameters errors are typically a result of improper client behavior and should be addressed by correcting the
request parameters before retrying.

## Example JSON
```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-parameters.md",
  "title": "Bad Parameters",
  "status": 400,
  "code": "400-02",
  "detail": "The request parameters are invalid or malformed",
  "instance": "/api/resource",
  "violations": [
    {
      "parameter": "sort",
      "detail": "The field 'sort' should be asc or desc",
      "type": "query"
    }
  ]
}
```
