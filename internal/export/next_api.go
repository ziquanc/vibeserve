package export

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// GenerateNextAPIClient generates a typed fetch wrapper (lib/api.ts) with
// CRUD functions for each table defined in the manifest.
func GenerateNextAPIClient(m *manifest.Manifest) string {
	var b strings.Builder

	b.WriteString("// Auto-generated API client — do not edit manually.\n")

	// Build import line with all types
	var typeImports []string
	typeImports = append(typeImports, "ListOptions")
	for _, s := range m.Schemas {
		name := TableToStructName(s.Table)
		typeImports = append(typeImports, name)
		typeImports = append(typeImports, "Create"+name+"Input")
		typeImports = append(typeImports, "Update"+name+"Input")
	}
	b.WriteString(fmt.Sprintf("import type { %s } from './types';\n\n", strings.Join(typeImports, ", ")))

	// fetchAPI helper
	b.WriteString(`const BASE = '/api';

async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  });
  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(error.error || res.statusText);
  }
  return res.json();
}

`)

	// Per-table CRUD functions
	for _, schema := range m.Schemas {
		name := TableToStructName(schema.Table)
		plural := name + "s"
		table := schema.Table

		b.WriteString(fmt.Sprintf("// %s\n\n", plural))

		// list
		b.WriteString(fmt.Sprintf("export async function list%s(options?: ListOptions): Promise<%s[]> {\n", plural, name))
		b.WriteString("  const params = new URLSearchParams();\n")
		b.WriteString("  if (options?.limit) params.set('limit', String(options.limit));\n")
		b.WriteString("  if (options?.offset) params.set('offset', String(options.offset));\n")
		b.WriteString("  if (options?.sort) params.set('sort', options.sort);\n")
		b.WriteString("  if (options?.order) params.set('order', options.order);\n")
		b.WriteString("  if (options?.search) params.set('search', options.search);\n")
		b.WriteString("  const qs = params.toString();\n")
		b.WriteString(fmt.Sprintf("  return fetchAPI<%s[]>(`/%s${qs ? '?' + qs : ''}`);\n", name, table))
		b.WriteString("}\n\n")

		// get
		b.WriteString(fmt.Sprintf("export async function get%s(id: number): Promise<%s> {\n", name, name))
		b.WriteString(fmt.Sprintf("  return fetchAPI<%s>(`/%s/${id}`);\n", name, table))
		b.WriteString("}\n\n")

		// create
		b.WriteString(fmt.Sprintf("export async function create%s(data: Create%sInput): Promise<%s> {\n", name, name, name))
		b.WriteString(fmt.Sprintf("  return fetchAPI<%s>('/%s', {\n", name, table))
		b.WriteString("    method: 'POST',\n")
		b.WriteString("    body: JSON.stringify(data),\n")
		b.WriteString("  });\n")
		b.WriteString("}\n\n")

		// update
		b.WriteString(fmt.Sprintf("export async function update%s(id: number, data: Update%sInput): Promise<%s> {\n", name, name, name))
		b.WriteString(fmt.Sprintf("  return fetchAPI<%s>(`/%s/${id}`, {\n", name, table))
		b.WriteString("    method: 'PUT',\n")
		b.WriteString("    body: JSON.stringify(data),\n")
		b.WriteString("  });\n")
		b.WriteString("}\n\n")

		// delete
		b.WriteString(fmt.Sprintf("export async function delete%s(id: number): Promise<void> {\n", name))
		b.WriteString(fmt.Sprintf("  await fetchAPI(`/%s/${id}`, { method: 'DELETE' });\n", table))
		b.WriteString("}\n\n")
	}

	return b.String()
}
