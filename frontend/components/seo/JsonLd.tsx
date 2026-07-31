interface JsonLdProps {
  data: object
}

/**
 * Escape a JSON payload for embedding inside an HTML `<script>` element.
 *
 * `JSON.stringify` escapes `"` and `\` — it does NOT escape `<`, so a `</script>`
 * sequence anywhere in the data terminates the block and everything after it
 * parses as HTML. Entity names on this site are user-contributed and reach this
 * component: `POST /shows` takes a free-text artist name from any authenticated
 * account and auto-approves the show, after which the name is served by
 * `GET /artists` and rendered into the `ItemList` on /artists. Without this,
 * that is stored XSS, and `script-src` includes `'unsafe-inline'` so the CSP
 * does not stop it.
 *
 * `\uXXXX` escapes are valid JSON and decode to the same characters, so the
 * structured data a crawler parses is unchanged. U+2028/U+2029 are legal in
 * JSON strings but are line terminators in JavaScript source, which breaks the
 * script for any consumer that evaluates rather than parses it.
 */
function escapeForScriptElement(json: string): string {
  return json
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

/** Serialize + escape for `<script>` embedding. Exported for unit tests. */
export function serializeJsonLd(data: object): string {
  return escapeForScriptElement(JSON.stringify(data))
}

export function JsonLd({ data }: JsonLdProps) {
  return (
    <script
      type="application/ld+json"
      dangerouslySetInnerHTML={{ __html: serializeJsonLd(data) }}
    />
  )
}
