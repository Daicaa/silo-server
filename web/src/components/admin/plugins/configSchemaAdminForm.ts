import type { PluginAdminForm, PluginAdminFormField, PluginConfigSchema } from "@/api/types";

export function humanizeConfigKey(value: string) {
  return value
    .split("_")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

export function adminFormForConfigSchema(schema: PluginConfigSchema): PluginAdminForm | null {
  if (schema.admin_form?.fields?.length) return schema.admin_form;

  try {
    const parsed = JSON.parse(schema.json_schema) as {
      type?: string;
      required?: string[];
      properties?: Record<
        string,
        {
          type?: string;
          title?: string;
          description?: string;
          writeOnly?: boolean;
          format?: string;
        }
      >;
    };
    if (parsed.type !== "object" || !parsed.properties) return null;

    const fields = Object.entries(parsed.properties).map(
      ([key, property]): PluginAdminFormField | null => {
        const propertyType = property.type;
        if (!propertyType || !["string", "number", "integer", "boolean"].includes(propertyType)) {
          return null;
        }
        const secret = property.writeOnly === true || property.format === "password";
        const control =
          propertyType === "boolean"
            ? "SWITCH"
            : propertyType === "number" || propertyType === "integer"
              ? "NUMBER"
              : secret
                ? "PASSWORD"
                : "TEXT";
        return {
          key,
          label: property.title || humanizeConfigKey(key),
          description: property.description,
          control,
          placeholder: "",
          required: parsed.required?.includes(key) ?? false,
          secret,
          multiline: false,
          options: [],
          rows: 0,
        };
      },
    );
    if (fields.some((field) => field == null)) return null;

    return {
      ...schema.admin_form,
      fields: fields.filter((field): field is PluginAdminFormField => field != null),
    };
  } catch {
    return null;
  }
}
