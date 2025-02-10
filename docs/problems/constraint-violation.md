# Constraint Violation

**Type**: `https://github.com/nickbryan/httputil/blob/main/docs/problems/constraint-violation.md`  
**Status**: `422 Unprocessable Entity`
**Code**: `422-02`

## Description

This error type is used when one or more validation rules or constraints are violated during request processing. Such constraints could include field level requirements (e.g., "required" fields), input formats, or invalid data ranges.

Constraint Violations occur when a client provides incorrect data that cannot be processed by the server. The `violations` field details each specific problem to help guide the client in correcting the issue.

## Example JSON

```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/constraint-violation.md",
  "title": "Constraint Violation",
  "status": 422,
  "code": "422-02",
  "detail": "The request data violated one or more validation constraints",
  "instance": "/api/resource",
  "extensions": {
    "violations": [
      {
        "detail": "The field 'email' is required.",
        "pointer": "/email"
      },
      {
        "detail": "The field 'age' must be a positive integer.",
        "pointer": "/age"
      }
    ]
  }
}
```
