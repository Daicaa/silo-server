import { describe, expect, it } from "vitest";

import type { PluginConfigSchema } from "@/api/types";

import {
  activeConnectionSchemas,
  buildConnectionConfig,
  connectionSchemasAreValid,
} from "./watchProviderConnectionConfig";

const optionalSchema: PluginConfigSchema = {
  key: "optional",
  title: "Optional server settings",
  description: "Only sent when configured.",
  json_schema: JSON.stringify({
    type: "object",
    properties: { base_url: { type: "string" } },
    required: ["base_url"],
  }),
  required: false,
  admin_form: {
    fields: [
      {
        key: "base_url",
        label: "Server URL",
        control: "TEXT",
        required: true,
        secret: false,
        multiline: false,
      },
    ],
  },
};

describe("watch provider connection config", () => {
  it("omits an untouched optional schema without blocking Connect", () => {
    expect(activeConnectionSchemas([optionalSchema], {})).toEqual([]);
    expect(connectionSchemasAreValid([optionalSchema], {}, { optional: false })).toBe(true);
    expect(buildConnectionConfig([optionalSchema], {})).toEqual({});
  });

  it("validates and submits an optional schema after the user enters a value", () => {
    const drafts = { optional: { base_url: "https://floppy.example.com" } };
    expect(connectionSchemasAreValid([optionalSchema], drafts, { optional: false })).toBe(false);
    expect(connectionSchemasAreValid([optionalSchema], drafts, { optional: true })).toBe(true);
    expect(buildConnectionConfig([optionalSchema], drafts)).toEqual({
      optional: { base_url: "https://floppy.example.com" },
    });
  });

  it("keeps required schemas active before any values are entered", () => {
    const requiredSchema = { ...optionalSchema, key: "required", required: true };
    expect(activeConnectionSchemas([requiredSchema], {})).toEqual([requiredSchema]);
    expect(connectionSchemasAreValid([requiredSchema], {}, {})).toBe(false);
  });
});
