package export

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// lucideIcons maps common table names to lucide-react icon names.
var lucideIcons = map[string]string{
	"users":         "Users",
	"posts":         "FileText",
	"comments":      "MessageCircle",
	"products":      "Package",
	"orders":        "ShoppingCart",
	"categories":    "FolderTree",
	"tags":          "Tag",
	"tasks":         "CheckSquare",
	"projects":      "Briefcase",
	"messages":      "Mail",
	"files":         "File",
	"images":        "Image",
	"settings":      "Settings",
	"roles":         "Shield",
	"permissions":   "Lock",
	"notifications": "Bell",
	"events":        "Calendar",
	"payments":      "CreditCard",
	"invoices":      "Receipt",
	"reviews":       "Star",
}

// iconForTable returns the lucide-react icon name for a table, defaulting to "Database".
func iconForTable(table string) string {
	if icon, ok := lucideIcons[table]; ok {
		return icon
	}
	return "Database"
}

// GenerateNextLayout generates the root layout.tsx with sidebar integration.
func GenerateNextLayout(m *manifest.Manifest) string {
	description := m.Description
	if description == "" {
		description = "Admin Panel"
	}

	return fmt.Sprintf(`import type { Metadata } from 'next';
import './globals.css';
import { Sidebar } from '@/components/sidebar';

export const metadata: Metadata = {
  title: '%s',
  description: '%s',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="flex min-h-screen bg-background text-foreground">
        <Sidebar />
        <main className="flex-1 p-8 overflow-auto">{children}</main>
      </body>
    </html>
  );
}
`, m.Name, description)
}

// GenerateNextSidebar generates the sidebar.tsx navigation component.
func GenerateNextSidebar(m *manifest.Manifest) string {
	var b strings.Builder

	// Collect unique icons needed
	iconSet := map[string]bool{"LayoutDashboard": true}
	type link struct {
		href  string
		label string
		icon  string
	}
	var links []link
	for _, s := range m.Schemas {
		icon := iconForTable(s.Table)
		iconSet[icon] = true
		links = append(links, link{
			href:  "/" + s.Table,
			label: PascalCase(s.Table),
			icon:  icon,
		})
	}

	// Sort icon names for deterministic imports
	var icons []string
	for icon := range iconSet {
		icons = append(icons, icon)
	}
	sort.Strings(icons)

	b.WriteString("'use client';\n\n")
	b.WriteString("import Link from 'next/link';\n")
	b.WriteString("import { usePathname } from 'next/navigation';\n")
	b.WriteString("import { cn } from '@/lib/utils';\n")
	b.WriteString(fmt.Sprintf("import { %s } from 'lucide-react';\n\n", strings.Join(icons, ", ")))

	b.WriteString("export function Sidebar() {\n")
	b.WriteString("  const pathname = usePathname();\n\n")

	// Build links array
	b.WriteString("  const links = [\n")
	b.WriteString("    { href: '/', label: 'Dashboard', icon: LayoutDashboard },\n")
	for _, l := range links {
		b.WriteString(fmt.Sprintf("    { href: '%s', label: '%s', icon: %s },\n", l.href, l.label, l.icon))
	}
	b.WriteString("  ];\n\n")

	b.WriteString(fmt.Sprintf("  return (\n    <aside className=\"w-64 border-r bg-muted/40 p-6\">\n      <h1 className=\"text-xl font-bold mb-8\">%s</h1>\n      <nav className=\"space-y-1\">\n        {links.map((link) => (\n          <Link\n            key={link.href}\n            href={link.href}\n            className={cn(\n              'flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors',\n              pathname === link.href\n                ? 'bg-primary text-primary-foreground'\n                : 'hover:bg-muted'\n            )}\n          >\n            <link.icon className=\"h-4 w-4\" />\n            {link.label}\n          </Link>\n        ))}\n      </nav>\n    </aside>\n  );\n}\n", m.Name))

	return b.String()
}

// GenerateNextDashboard generates the dashboard page.tsx with resource cards.
func GenerateNextDashboard(m *manifest.Manifest) string {
	var b strings.Builder

	// Collect unique icons
	iconSet := make(map[string]bool)
	type resource struct {
		name        string
		href        string
		icon        string
		description string
	}
	var resources []resource
	for _, s := range m.Schemas {
		icon := iconForTable(s.Table)
		iconSet[icon] = true
		resources = append(resources, resource{
			name:        PascalCase(s.Table),
			href:        "/" + s.Table,
			icon:        icon,
			description: fmt.Sprintf("%d columns", len(s.Columns)),
		})
	}

	var icons []string
	for icon := range iconSet {
		icons = append(icons, icon)
	}
	sort.Strings(icons)

	description := m.Description
	if description == "" {
		description = "Manage your data"
	}

	b.WriteString("'use client';\n\n")
	b.WriteString("import Link from 'next/link';\n")
	b.WriteString("import { Card, CardHeader, CardTitle } from '@/components/ui/card';\n")
	b.WriteString(fmt.Sprintf("import { %s } from 'lucide-react';\n\n", strings.Join(icons, ", ")))

	// Build resources array
	b.WriteString("const resources = [\n")
	for _, r := range resources {
		b.WriteString(fmt.Sprintf("  { name: '%s', href: '%s', icon: %s, description: '%s' },\n", r.name, r.href, r.icon, r.description))
	}
	b.WriteString("];\n\n")

	b.WriteString("export default function Dashboard() {\n")
	b.WriteString("  return (\n")
	b.WriteString("    <div>\n")
	b.WriteString(fmt.Sprintf("      <h1 className=\"text-3xl font-bold mb-8\">%s</h1>\n", m.Name))
	b.WriteString(fmt.Sprintf("      <p className=\"text-muted-foreground mb-8\">%s</p>\n", description))
	b.WriteString("      <div className=\"grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6\">\n")
	b.WriteString("        {resources.map((r) => (\n")
	b.WriteString("          <Link key={r.href} href={r.href}>\n")
	b.WriteString("            <Card className=\"hover:shadow-md transition-shadow cursor-pointer\">\n")
	b.WriteString("              <CardHeader className=\"flex flex-row items-center gap-4\">\n")
	b.WriteString("                <r.icon className=\"h-8 w-8 text-primary\" />\n")
	b.WriteString("                <div>\n")
	b.WriteString("                  <CardTitle>{r.name}</CardTitle>\n")
	b.WriteString("                  <p className=\"text-sm text-muted-foreground\">{r.description}</p>\n")
	b.WriteString("                </div>\n")
	b.WriteString("              </CardHeader>\n")
	b.WriteString("            </Card>\n")
	b.WriteString("          </Link>\n")
	b.WriteString("        ))}\n")
	b.WriteString("      </div>\n")
	b.WriteString("    </div>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return b.String()
}
