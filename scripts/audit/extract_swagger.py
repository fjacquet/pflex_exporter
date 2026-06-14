#!/usr/bin/env python3
"""Extract paths and response schema field names from in-scope PowerFlex swagger specs.

Usage: python3 scripts/audit/extract_swagger.py docs/swagger/<file>.json
Prints: every path+method, and the flattened set of response property names per 2xx schema.
"""
import json
import sys


def schema_fields(schema, defs, seen=None):
    """Recursively collect property names from a (possibly $ref) schema."""
    if seen is None:
        seen = set()
    fields = set()
    if not isinstance(schema, dict):
        return fields
    ref = schema.get("$ref")
    if ref:
        # Support both Swagger 2.0 (#/definitions/Foo) and
        # OpenAPI 3.x (#/components/schemas/Foo or #/components/schemas/foo.yaml)
        name = ref.split("/")[-1]
        if name in seen:
            return fields
        seen.add(name)
        return schema_fields(defs.get(name, {}), defs, seen)
    for key, val in schema.get("properties", {}).items():
        fields.add(key)
        fields |= schema_fields(val, defs, seen)
    if "items" in schema:
        fields |= schema_fields(schema["items"], defs, seen)
    # Handle allOf / anyOf / oneOf composition
    for combiner in ("allOf", "anyOf", "oneOf"):
        for sub in schema.get(combiner, []):
            fields |= schema_fields(sub, defs, seen)
    return fields


def main(path):
    with open(path) as f:
        spec = json.load(f)

    # Swagger 2.0 uses "definitions"; OpenAPI 3.x uses "components.schemas"
    defs = spec.get("definitions") or spec.get("components", {}).get("schemas", {})

    print(f"# {path} :: {spec.get('info', {}).get('title', '?')}")
    for p, methods in sorted(spec.get("paths", {}).items()):
        for method, op in methods.items():
            if method not in ("get", "post", "put", "delete", "patch"):
                continue
            print(f"\n{method.upper()} {p}")
            params = [pr.get("name") for pr in op.get("parameters", []) if pr.get("name")]
            if params:
                print(f"  params: {sorted(params)}")
            resp = op.get("responses", {})
            for code in ("200", "201"):
                r = resp.get(code, {})
                # Swagger 2.0: response.schema
                sch = r.get("schema")
                # OpenAPI 3.x: response.content.application/json.schema
                if sch is None:
                    sch = (
                        r.get("content", {})
                        .get("application/json", {})
                        .get("schema")
                    )
                if sch:
                    fields = sorted(schema_fields(sch, defs))
                    print(f"  {code} fields: {fields}")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <swagger-file.json>", file=sys.stderr)
        sys.exit(1)
    main(sys.argv[1])
