/**
 * Promptfoo prompt function for the /ingest extraction eval.
 *
 * Builds the multimodal Anthropic message from TWO sources so the eval never
 * drifts from the skill:
 *   - the extraction text from `extraction-prompt.md` (the single source of
 *     truth, shared with the /ingest skill)
 *   - the fixture image, supplied as a base64 test var `image`
 *     (promptfoo auto-base64-encodes a `file://...png` var)
 *
 * Promptfoo calls this with ({ vars, provider }) and expects an array of
 * Anthropic message objects. See https://www.promptfoo.dev/docs/configuration/prompts/
 */
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const HERE = dirname(fileURLToPath(import.meta.url));

/** The extraction instructions live in extraction-prompt.md, after the "## Prompt" heading. */
function loadExtractionInstructions(): string {
  const raw = readFileSync(join(HERE, "extraction-prompt.md"), "utf-8");
  const marker = "## Prompt";
  const idx = raw.indexOf(marker);
  if (idx === -1) {
    throw new Error("extraction-prompt.md is missing the '## Prompt' section");
  }
  return raw.slice(idx + marker.length).trim();
}

interface PromptVars {
  image?: string;
  media_type?: string;
}

export default function buildPrompt({ vars }: { vars: PromptVars }) {
  const instructions = loadExtractionInstructions();
  const image = vars.image;
  if (!image) {
    throw new Error("prompt var `image` is required (point it at a file:// image)");
  }

  return [
    {
      role: "user",
      content: [
        { type: "text", text: instructions },
        {
          type: "image",
          source: {
            type: "base64",
            media_type: vars.media_type ?? "image/png",
            data: image,
          },
        },
      ],
    },
  ];
}
