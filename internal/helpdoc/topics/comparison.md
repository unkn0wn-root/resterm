# Compare Runs

Run the same request across environments and compare responses side by side.

```http
# @compare dev stage prod base=dev
GET {{apiUrl}}/health
```

Press `g c` to start a compare sweep. The Compare tab lists status and duration per environment, and `Enter` loads a row against the pinned baseline for diffing.
