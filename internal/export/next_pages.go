package export

import (
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// titleCase converts a snake_case string to Title Case with spaces.
// "user_id" → "User Id", "created_at" → "Created At"
func titleCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// inputType returns the HTML input type for a column type.
func inputType(colType string) string {
	switch colType {
	case "INTEGER", "REAL":
		return "number"
	case "BOOLEAN":
		return "checkbox"
	case "DATE":
		return "date"
	case "DATETIME":
		return "datetime-local"
	default:
		return "text"
	}
}

// isTimestamp returns true if the column is a timestamp field.
func isTimestamp(name string) bool {
	return name == "created_at" || name == "updated_at" || name == "deleted_at"
}

// GenerateNextResourcePages generates 4 CRUD pages (list, create, detail, edit)
// for a given schema table. Returns a map of filename -> TSX content.
func GenerateNextResourcePages(schema manifest.Schema, table string) map[string]string {
	pages := make(map[string]string)

	schema = manifest.InjectTimestamps(schema)

	name := TableToStructName(table) // "User"
	plural := name + "s"            // "Users"

	// Filter columns for different views
	var displayCols, formCols, detailCols []manifest.Column
	for _, c := range schema.Columns {
		if c.Name == "deleted_at" {
			continue
		}
		// Detail page shows all columns (including timestamps)
		detailCols = append(detailCols, c)
		// List page shows non-timestamp columns
		if !isTimestamp(c.Name) {
			displayCols = append(displayCols, c)
		}
		// Form pages exclude PK auto and timestamps
		if c.Primary && c.Auto {
			continue
		}
		if isTimestamp(c.Name) {
			continue
		}
		formCols = append(formCols, c)
	}

	pages[table+"/page.tsx"] = generateListPage(table, name, plural, displayCols)
	pages[table+"/new/page.tsx"] = generateCreatePage(table, name, plural, formCols)
	pages[table+"/[id]/page.tsx"] = generateDetailPage(table, name, plural, detailCols)
	pages[table+"/[id]/edit/page.tsx"] = generateEditPage(table, name, plural, formCols)

	return pages
}

// generateListPage creates the list/index page with search, sort, pagination.
func generateListPage(table, name, plural string, cols []manifest.Column) string {
	var b strings.Builder

	b.WriteString("'use client';\n\n")
	b.WriteString("import { useState, useEffect, useCallback } from 'react';\n")
	b.WriteString("import Link from 'next/link';\n")
	b.WriteString("import { useRouter } from 'next/navigation';\n")
	b.WriteString("import { Button } from '@/components/ui/button';\n")
	b.WriteString("import { Input } from '@/components/ui/input';\n")
	b.WriteString("import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table';\n")
	b.WriteString(fmt.Sprintf("import { list%s, delete%s } from '@/lib/api';\n", plural, name))
	b.WriteString(fmt.Sprintf("import type { %s } from '@/lib/types';\n", name))
	b.WriteString("import { Plus, Pencil, Trash2, ArrowUpDown } from 'lucide-react';\n\n")

	b.WriteString(fmt.Sprintf("export default function %sPage() {\n", plural))
	b.WriteString("  const router = useRouter();\n")
	b.WriteString(fmt.Sprintf("  const [data, setData] = useState<%s[]>([]);\n", name))
	b.WriteString("  const [search, setSearch] = useState('');\n")
	b.WriteString("  const [sort, setSort] = useState('id');\n")
	b.WriteString("  const [order, setOrder] = useState<'asc' | 'desc'>('asc');\n")
	b.WriteString("  const [page, setPage] = useState(0);\n")
	b.WriteString("  const limit = 20;\n\n")

	b.WriteString("  const load = useCallback(async () => {\n")
	b.WriteString("    try {\n")
	b.WriteString(fmt.Sprintf("      const result = await list%s({ limit, offset: page * limit, sort, order, search: search || undefined });\n", plural))
	b.WriteString("      setData(result);\n")
	b.WriteString("    } catch (err) {\n")
	b.WriteString("      console.error(err);\n")
	b.WriteString("    }\n")
	b.WriteString("  }, [page, sort, order, search]);\n\n")

	b.WriteString("  useEffect(() => { load(); }, [load]);\n\n")

	b.WriteString("  const handleSort = (col: string) => {\n")
	b.WriteString("    if (sort === col) { setOrder(order === 'asc' ? 'desc' : 'asc'); }\n")
	b.WriteString("    else { setSort(col); setOrder('asc'); }\n")
	b.WriteString("  };\n\n")

	b.WriteString("  const handleDelete = async (id: number) => {\n")
	b.WriteString("    if (!confirm('Delete this item?')) return;\n")
	b.WriteString(fmt.Sprintf("    await delete%s(id);\n", name))
	b.WriteString("    load();\n")
	b.WriteString("  };\n\n")

	b.WriteString("  return (\n")
	b.WriteString("    <div>\n")
	b.WriteString("      <div className=\"flex items-center justify-between mb-6\">\n")
	b.WriteString(fmt.Sprintf("        <h1 className=\"text-2xl font-bold\">%s</h1>\n", plural))
	b.WriteString(fmt.Sprintf("        <Link href=\"/%s/new\"><Button><Plus className=\"h-4 w-4 mr-2\" />New %s</Button></Link>\n", table, name))
	b.WriteString("      </div>\n")
	b.WriteString("      <div className=\"mb-4\">\n")
	b.WriteString("        <Input placeholder=\"Search...\" value={search} onChange={(e) => { setSearch(e.target.value); setPage(0); }} />\n")
	b.WriteString("      </div>\n")
	b.WriteString("      <Table>\n")
	b.WriteString("        <TableHeader>\n")
	b.WriteString("          <TableRow>\n")

	// Column headers
	for _, c := range cols {
		b.WriteString(fmt.Sprintf("            <TableHead onClick={() => handleSort('%s')} className=\"cursor-pointer\">%s <ArrowUpDown className=\"inline h-3 w-3\" /></TableHead>\n", c.Name, titleCase(c.Name)))
	}
	b.WriteString("            <TableHead>Actions</TableHead>\n")

	b.WriteString("          </TableRow>\n")
	b.WriteString("        </TableHeader>\n")
	b.WriteString("        <TableBody>\n")
	b.WriteString("          {data.map((row) => (\n")
	b.WriteString("            <TableRow key={row.id}>\n")

	// Column cells
	for _, c := range cols {
		if c.Type == "BOOLEAN" {
			b.WriteString(fmt.Sprintf("              <TableCell>{row.%s ? 'Yes' : 'No'}</TableCell>\n", c.Name))
		} else {
			b.WriteString(fmt.Sprintf("              <TableCell>{row.%s}</TableCell>\n", c.Name))
		}
	}

	b.WriteString("              <TableCell>\n")
	b.WriteString("                <div className=\"flex gap-2\">\n")
	b.WriteString(fmt.Sprintf("                  <Button variant=\"ghost\" size=\"icon\" onClick={() => router.push(`/%s/${row.id}/edit`)}><Pencil className=\"h-4 w-4\" /></Button>\n", table))
	b.WriteString("                  <Button variant=\"ghost\" size=\"icon\" onClick={() => handleDelete(row.id)}><Trash2 className=\"h-4 w-4\" /></Button>\n")
	b.WriteString("                </div>\n")
	b.WriteString("              </TableCell>\n")

	b.WriteString("            </TableRow>\n")
	b.WriteString("          ))}\n")
	b.WriteString("        </TableBody>\n")
	b.WriteString("      </Table>\n")
	b.WriteString("      <div className=\"flex justify-between mt-4\">\n")
	b.WriteString("        <Button variant=\"outline\" disabled={page === 0} onClick={() => setPage(p => p - 1)}>Previous</Button>\n")
	b.WriteString("        <Button variant=\"outline\" disabled={data.length < limit} onClick={() => setPage(p => p + 1)}>Next</Button>\n")
	b.WriteString("      </div>\n")
	b.WriteString("    </div>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return b.String()
}

// generateCreatePage creates the new resource form page.
func generateCreatePage(table, name, plural string, cols []manifest.Column) string {
	var b strings.Builder

	b.WriteString("'use client';\n\n")
	b.WriteString("import { useState } from 'react';\n")
	b.WriteString("import { useRouter } from 'next/navigation';\n")
	b.WriteString("import { Button } from '@/components/ui/button';\n")
	b.WriteString("import { Input } from '@/components/ui/input';\n")
	b.WriteString("import { Label } from '@/components/ui/label';\n")
	b.WriteString("import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';\n")
	b.WriteString(fmt.Sprintf("import { create%s } from '@/lib/api';\n\n", name))

	b.WriteString(fmt.Sprintf("export default function New%sPage() {\n", name))
	b.WriteString("  const router = useRouter();\n")

	// Build form state initializer
	b.WriteString("  const [form, setForm] = useState({ ")
	for i, c := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		if c.Type == "BOOLEAN" {
			b.WriteString(fmt.Sprintf("%s: false", c.Name))
		} else {
			b.WriteString(fmt.Sprintf("%s: ''", c.Name))
		}
	}
	b.WriteString(" });\n")
	b.WriteString("  const [error, setError] = useState('');\n\n")

	// Submit handler
	b.WriteString("  const handleSubmit = async (e: React.FormEvent) => {\n")
	b.WriteString("    e.preventDefault();\n")
	b.WriteString("    try {\n")
	b.WriteString(fmt.Sprintf("      await create%s({\n", name))
	for _, c := range cols {
		switch c.Type {
		case "INTEGER":
			b.WriteString(fmt.Sprintf("        %s: form.%s ? Number(form.%s) : undefined,\n", c.Name, c.Name, c.Name))
		case "REAL":
			b.WriteString(fmt.Sprintf("        %s: form.%s ? Number(form.%s) : undefined,\n", c.Name, c.Name, c.Name))
		case "BOOLEAN":
			b.WriteString(fmt.Sprintf("        %s: form.%s,\n", c.Name, c.Name))
		default:
			b.WriteString(fmt.Sprintf("        %s: form.%s,\n", c.Name, c.Name))
		}
	}
	b.WriteString("      });\n")
	b.WriteString(fmt.Sprintf("      router.push('/%s');\n", table))
	b.WriteString("    } catch (err: any) {\n")
	b.WriteString("      setError(err.message);\n")
	b.WriteString("    }\n")
	b.WriteString("  };\n\n")

	b.WriteString("  return (\n")
	b.WriteString("    <Card className=\"max-w-lg\">\n")
	b.WriteString(fmt.Sprintf("      <CardHeader><CardTitle>New %s</CardTitle></CardHeader>\n", name))
	b.WriteString("      <CardContent>\n")
	b.WriteString("        {error && <p className=\"text-destructive mb-4\">{error}</p>}\n")
	b.WriteString("        <form onSubmit={handleSubmit} className=\"space-y-4\">\n")

	// Form fields
	for _, c := range cols {
		label := titleCase(c.Name)
		if c.Required {
			label += " *"
		}
		iType := inputType(c.Type)

		if c.Type == "BOOLEAN" {
			b.WriteString(fmt.Sprintf("          <div className=\"flex items-center gap-2\">\n"))
			b.WriteString(fmt.Sprintf("            <Input id=\"%s\" type=\"checkbox\" className=\"w-4 h-4\" checked={form.%s} onChange={e => setForm({...form, %s: e.target.checked})} />\n", c.Name, c.Name, c.Name))
			b.WriteString(fmt.Sprintf("            <Label htmlFor=\"%s\">%s</Label>\n", c.Name, label))
			b.WriteString("          </div>\n")
		} else {
			b.WriteString(fmt.Sprintf("          <div>\n"))
			b.WriteString(fmt.Sprintf("            <Label htmlFor=\"%s\">%s</Label>\n", c.Name, label))
			b.WriteString(fmt.Sprintf("            <Input id=\"%s\" type=\"%s\" value={form.%s} onChange={e => setForm({...form, %s: e.target.value})}",
				c.Name, iType, c.Name, c.Name))
			if c.Required {
				b.WriteString(" required")
			}
			b.WriteString(" />\n")
			b.WriteString("          </div>\n")
		}
	}

	b.WriteString("          <div className=\"flex gap-4\">\n")
	b.WriteString("            <Button type=\"submit\">Create</Button>\n")
	b.WriteString("            <Button type=\"button\" variant=\"outline\" onClick={() => router.back()}>Cancel</Button>\n")
	b.WriteString("          </div>\n")
	b.WriteString("        </form>\n")
	b.WriteString("      </CardContent>\n")
	b.WriteString("    </Card>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return b.String()
}

// generateDetailPage creates the resource detail view page.
func generateDetailPage(table, name, plural string, cols []manifest.Column) string {
	var b strings.Builder

	b.WriteString("'use client';\n\n")
	b.WriteString("import { useState, useEffect } from 'react';\n")
	b.WriteString("import { useParams, useRouter } from 'next/navigation';\n")
	b.WriteString("import Link from 'next/link';\n")
	b.WriteString("import { Button } from '@/components/ui/button';\n")
	b.WriteString("import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';\n")
	b.WriteString(fmt.Sprintf("import { get%s, delete%s } from '@/lib/api';\n", name, name))
	b.WriteString(fmt.Sprintf("import type { %s } from '@/lib/types';\n", name))
	b.WriteString("import { Pencil, Trash2, ArrowLeft } from 'lucide-react';\n\n")

	b.WriteString(fmt.Sprintf("export default function %sDetailPage() {\n", name))
	b.WriteString("  const params = useParams();\n")
	b.WriteString("  const router = useRouter();\n")
	b.WriteString(fmt.Sprintf("  const [data, setData] = useState<%s | null>(null);\n\n", name))

	b.WriteString("  useEffect(() => {\n")
	b.WriteString(fmt.Sprintf("    get%s(Number(params.id)).then(setData).catch(() => router.push('/%s'));\n", name, table))
	b.WriteString("  }, [params.id, router]);\n\n")

	b.WriteString("  if (!data) return <p>Loading...</p>;\n\n")

	b.WriteString("  const handleDelete = async () => {\n")
	b.WriteString(fmt.Sprintf("    if (!confirm('Delete this %s?')) return;\n", strings.ToLower(name)))
	b.WriteString(fmt.Sprintf("    await delete%s(data.id);\n", name))
	b.WriteString(fmt.Sprintf("    router.push('/%s');\n", table))
	b.WriteString("  };\n\n")

	b.WriteString("  return (\n")
	b.WriteString("    <div>\n")
	b.WriteString("      <Button variant=\"ghost\" onClick={() => router.back()} className=\"mb-4\"><ArrowLeft className=\"h-4 w-4 mr-2\" />Back</Button>\n")
	b.WriteString("      <Card className=\"max-w-lg\">\n")
	b.WriteString("        <CardHeader className=\"flex flex-row items-center justify-between\">\n")
	b.WriteString(fmt.Sprintf("          <CardTitle>%s #{data.id}</CardTitle>\n", name))
	b.WriteString("          <div className=\"flex gap-2\">\n")
	b.WriteString(fmt.Sprintf("            <Link href={`/%s/${data.id}/edit`}><Button variant=\"outline\" size=\"sm\"><Pencil className=\"h-4 w-4 mr-1\" />Edit</Button></Link>\n", table))
	b.WriteString("            <Button variant=\"destructive\" size=\"sm\" onClick={handleDelete}><Trash2 className=\"h-4 w-4 mr-1\" />Delete</Button>\n")
	b.WriteString("          </div>\n")
	b.WriteString("        </CardHeader>\n")
	b.WriteString("        <CardContent>\n")
	b.WriteString("          <dl className=\"space-y-3\">\n")

	// Detail fields — skip id (shown in title) and deleted_at
	for _, c := range cols {
		if c.Name == "id" || c.Name == "deleted_at" {
			continue
		}
		label := titleCase(c.Name)
		if c.Type == "BOOLEAN" {
			b.WriteString(fmt.Sprintf("            <div><dt className=\"text-sm text-muted-foreground\">%s</dt><dd className=\"font-medium\">{data.%s ? 'Yes' : 'No'}</dd></div>\n", label, c.Name))
		} else {
			b.WriteString(fmt.Sprintf("            <div><dt className=\"text-sm text-muted-foreground\">%s</dt><dd className=\"font-medium\">{data.%s}</dd></div>\n", label, c.Name))
		}
	}

	b.WriteString("          </dl>\n")
	b.WriteString("        </CardContent>\n")
	b.WriteString("      </Card>\n")
	b.WriteString("    </div>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return b.String()
}

// generateEditPage creates the edit resource form page with pre-filled data.
func generateEditPage(table, name, plural string, cols []manifest.Column) string {
	var b strings.Builder

	b.WriteString("'use client';\n\n")
	b.WriteString("import { useState, useEffect } from 'react';\n")
	b.WriteString("import { useParams, useRouter } from 'next/navigation';\n")
	b.WriteString("import { Button } from '@/components/ui/button';\n")
	b.WriteString("import { Input } from '@/components/ui/input';\n")
	b.WriteString("import { Label } from '@/components/ui/label';\n")
	b.WriteString("import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';\n")
	b.WriteString(fmt.Sprintf("import { get%s, update%s } from '@/lib/api';\n\n", name, name))

	b.WriteString(fmt.Sprintf("export default function Edit%sPage() {\n", name))
	b.WriteString("  const params = useParams();\n")
	b.WriteString("  const router = useRouter();\n")

	// Build form state initializer
	b.WriteString("  const [form, setForm] = useState({ ")
	for i, c := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		if c.Type == "BOOLEAN" {
			b.WriteString(fmt.Sprintf("%s: false", c.Name))
		} else {
			b.WriteString(fmt.Sprintf("%s: ''", c.Name))
		}
	}
	b.WriteString(" });\n")
	b.WriteString("  const [error, setError] = useState('');\n\n")

	// Load existing data
	b.WriteString("  useEffect(() => {\n")
	b.WriteString(fmt.Sprintf("    get%s(Number(params.id)).then((data) => {\n", name))
	b.WriteString("      setForm({\n")
	for _, c := range cols {
		switch c.Type {
		case "BOOLEAN":
			b.WriteString(fmt.Sprintf("        %s: data.%s || false,\n", c.Name, c.Name))
		case "INTEGER", "REAL":
			b.WriteString(fmt.Sprintf("        %s: String(data.%s ?? ''),\n", c.Name, c.Name))
		default:
			b.WriteString(fmt.Sprintf("        %s: data.%s || '',\n", c.Name, c.Name))
		}
	}
	b.WriteString("      });\n")
	b.WriteString("    });\n")
	b.WriteString("  }, [params.id]);\n\n")

	// Submit handler
	b.WriteString("  const handleSubmit = async (e: React.FormEvent) => {\n")
	b.WriteString("    e.preventDefault();\n")
	b.WriteString("    try {\n")
	b.WriteString(fmt.Sprintf("      await update%s(Number(params.id), {\n", name))
	for _, c := range cols {
		switch c.Type {
		case "INTEGER":
			b.WriteString(fmt.Sprintf("        %s: form.%s ? Number(form.%s) : undefined,\n", c.Name, c.Name, c.Name))
		case "REAL":
			b.WriteString(fmt.Sprintf("        %s: form.%s ? Number(form.%s) : undefined,\n", c.Name, c.Name, c.Name))
		case "BOOLEAN":
			b.WriteString(fmt.Sprintf("        %s: form.%s,\n", c.Name, c.Name))
		default:
			b.WriteString(fmt.Sprintf("        %s: form.%s,\n", c.Name, c.Name))
		}
	}
	b.WriteString("      });\n")
	b.WriteString(fmt.Sprintf("      router.push(`/%s/${params.id}`);\n", table))
	b.WriteString("    } catch (err: any) {\n")
	b.WriteString("      setError(err.message);\n")
	b.WriteString("    }\n")
	b.WriteString("  };\n\n")

	b.WriteString("  return (\n")
	b.WriteString("    <Card className=\"max-w-lg\">\n")
	b.WriteString(fmt.Sprintf("      <CardHeader><CardTitle>Edit %s</CardTitle></CardHeader>\n", name))
	b.WriteString("      <CardContent>\n")
	b.WriteString("        {error && <p className=\"text-destructive mb-4\">{error}</p>}\n")
	b.WriteString("        <form onSubmit={handleSubmit} className=\"space-y-4\">\n")

	// Form fields (same as create but pre-filled)
	for _, c := range cols {
		label := titleCase(c.Name)
		if c.Required {
			label += " *"
		}
		iType := inputType(c.Type)

		if c.Type == "BOOLEAN" {
			b.WriteString("          <div className=\"flex items-center gap-2\">\n")
			b.WriteString(fmt.Sprintf("            <Input id=\"%s\" type=\"checkbox\" className=\"w-4 h-4\" checked={form.%s} onChange={e => setForm({...form, %s: e.target.checked})} />\n", c.Name, c.Name, c.Name))
			b.WriteString(fmt.Sprintf("            <Label htmlFor=\"%s\">%s</Label>\n", c.Name, label))
			b.WriteString("          </div>\n")
		} else {
			b.WriteString("          <div>\n")
			b.WriteString(fmt.Sprintf("            <Label htmlFor=\"%s\">%s</Label>\n", c.Name, label))
			b.WriteString(fmt.Sprintf("            <Input id=\"%s\" type=\"%s\" value={form.%s} onChange={e => setForm({...form, %s: e.target.value})}",
				c.Name, iType, c.Name, c.Name))
			if c.Required {
				b.WriteString(" required")
			}
			b.WriteString(" />\n")
			b.WriteString("          </div>\n")
		}
	}

	b.WriteString("          <div className=\"flex gap-4\">\n")
	b.WriteString("            <Button type=\"submit\">Save</Button>\n")
	b.WriteString("            <Button type=\"button\" variant=\"outline\" onClick={() => router.back()}>Cancel</Button>\n")
	b.WriteString("          </div>\n")
	b.WriteString("        </form>\n")
	b.WriteString("      </CardContent>\n")
	b.WriteString("    </Card>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return b.String()
}
