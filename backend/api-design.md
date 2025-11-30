## Request

POST /show

### payload

```json
{
    artists: [
        name
    ],
    venue,
    date,
    cost,
    ages,
    city,
    state,
    description
}
```

### response

```json
{
  success
}
```

## Request

POST /show/ai-suggestion

### payload

```json
{
    prompt
}
```

### response

```json
{
    success,
    artists: [
        name
    ],
    venue,
    date,
    cost,
    ages,
    city,
    state,
    description
}
```
