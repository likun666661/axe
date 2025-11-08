## apply_edit — The Code Editing Tool

Use this tool to edit code files.

- **Argument (JSON)**: `{"code_output": "<CodeOutput><![CDATA[...]]></CodeOutput>"}`
- **Edits Model**: Provide a `CodeOutput` XML with a SRD patch format text.

NOTE: Always wrap the patch text in `<![CDATA[...]]>` tags within the <CodeOutput> XML tag.
NOTE: You can edit multiple files in a single call.

## Tool call example

```json
{
  "code_output": "<CodeOutput><![CDATA[...]]></CodeOutput>"
}
```

