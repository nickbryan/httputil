# Business Rule Violation
**Type**: [https://github.com/nickbryan/httputil/blob/main/docs/problems/business-rule-violation.md](https://github.com/nickbryan/httputil/blob/main/docs/problems/business-rule-violation.md)  
**Status**: `422 Unprocessable Entity`  
**Code**: `422-01`

## Description
This error type is used when one or more business rules are violated during request processing. Business rules 
ensure that the system functions in accordance with organizational policies or domain-specific constraints.

Business Rule Violations occur when a request is structurally valid but fails to meet logical or policy-based 
conditions enforced by the application. The `violations` field outlines each specific violation to provide 
guidance for modifying the request to comply with the business rules.

## Example JSON
```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/business-rule-violation.md",
  "title": "Business Rule Violation",
  "status": 422,
  "code": "422-01",
  "detail": "The request violates one or more business rules",
  "instance": "/api/resource",
  "violations": [
    {
      "detail": "The customer is not eligible for this promotion",
      "pointer": "/customer/eligibility"
    },
    {
      "detail": "The transaction exceeds the maximum allowed limit",
      "pointer": "/transaction/amount"
    }
  ]
}
```
