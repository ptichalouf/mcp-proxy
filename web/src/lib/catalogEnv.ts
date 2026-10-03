import type { CatalogItem, CatalogEnvVar } from '../types';

export interface CatalogEnvField {
  key: string;
  value: string;
  description: string;
  required: boolean;
  isSecret: boolean;
}

// The Go catalog serializes requirements under `env`, not `envRequirements`.
export function catalogEnvRequirements(item: CatalogItem | null): CatalogEnvField[] {
  if (!item) return [];
  const metadata = item.env ?? item.envRequirements ?? [];
  const values = item.defaultConfig?.env ?? {};
  const fields = metadata.map((req: CatalogEnvVar) => {
    const key = req.name ?? req.key ?? '';
    return {
      key,
      value: values[key] ?? req.default ?? '',
      description: req.description,
      required: req.required,
      isSecret: Boolean(req.isSecret ?? req.secret ?? /key|token|secret|password/i.test(key)),
    };
  }).filter((field) => field.key);
  if (fields.length) return fields;
  return Object.entries(values).map(([key, value]) => ({
    key, value, description: '', required: false,
    isSecret: /key|token|secret|password/i.test(key),
  }));
}

export function missingRequiredEnv(fields: CatalogEnvField[], supplied?: Record<string, string>): string[] {
  return fields.filter((field) => field.required && !(supplied ? supplied[field.key] : field.value)?.trim()).map((field) => field.key);
}
