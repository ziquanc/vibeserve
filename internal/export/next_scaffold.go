package export

import "encoding/json"

// GenerateNextPackageJSON creates a package.json for a Next.js frontend project.
func GenerateNextPackageJSON(name string) string {
	type PackageJSON struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Private         bool              `json:"private"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}

	pkg := PackageJSON{
		Name:    Slugify(name),
		Version: "0.1.0",
		Private: true,
		Scripts: map[string]string{
			"dev":   "next dev",
			"build": "next build",
			"start": "next start",
			"lint":  "next lint",
		},
		Dependencies: map[string]string{
			"next":                     "^15.0.0",
			"react":                    "^19.0.0",
			"react-dom":               "^19.0.0",
			"clsx":                     "^2.1.0",
			"tailwind-merge":           "^2.5.0",
			"class-variance-authority": "^0.7.0",
			"lucide-react":             "^0.400.0",
		},
		DevDependencies: map[string]string{
			"typescript":         "^5.5.0",
			"@types/node":       "^20.0.0",
			"@types/react":      "^19.0.0",
			"@types/react-dom":  "^19.0.0",
			"tailwindcss":       "^4.0.0",
			"@tailwindcss/postcss": "^4.0.0",
			"postcss":           "^8.0.0",
		},
	}

	data, _ := json.MarshalIndent(pkg, "", "  ")
	return string(data) + "\n"
}

// GenerateNextConfig creates next.config.ts with API proxy rewrites.
func GenerateNextConfig() string {
	return `import type { NextConfig } from 'next';

const config: NextConfig = {
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: ` + "`" + `${process.env.API_URL || 'http://localhost:8080'}/:path*` + "`" + `,
      },
    ];
  },
};

export default config;
`
}

// GenerateNextTSConfig creates a tsconfig.json for a Next.js project.
func GenerateNextTSConfig() string {
	type Plugin struct {
		Name string `json:"name"`
	}
	type CompilerOptions struct {
		Target            string            `json:"target"`
		Lib               []string          `json:"lib"`
		AllowJS           bool              `json:"allowJs"`
		SkipLibCheck      bool              `json:"skipLibCheck"`
		Strict            bool              `json:"strict"`
		NoEmit            bool              `json:"noEmit"`
		ESModuleInterop   bool              `json:"esModuleInterop"`
		Module            string            `json:"module"`
		ModuleResolution  string            `json:"moduleResolution"`
		ResolveJSON       bool              `json:"resolveJsonModule"`
		IsolatedModules   bool              `json:"isolatedModules"`
		JSX               string            `json:"jsx"`
		Incremental       bool              `json:"incremental"`
		Plugins           []Plugin          `json:"plugins"`
		Paths             map[string][]string `json:"paths"`
	}
	type TSConfig struct {
		CompilerOptions CompilerOptions `json:"compilerOptions"`
		Include         []string        `json:"include"`
		Exclude         []string        `json:"exclude"`
	}

	cfg := TSConfig{
		CompilerOptions: CompilerOptions{
			Target:           "ES2017",
			Lib:              []string{"dom", "dom.iterable", "esnext"},
			AllowJS:          true,
			SkipLibCheck:     true,
			Strict:           true,
			NoEmit:           true,
			ESModuleInterop:  true,
			Module:           "esnext",
			ModuleResolution: "bundler",
			ResolveJSON:      true,
			IsolatedModules:  true,
			JSX:              "preserve",
			Incremental:      true,
			Plugins:          []Plugin{{Name: "next"}},
			Paths:            map[string][]string{"@/*": {"./src/*"}},
		},
		Include: []string{"next-env.d.ts", "**/*.ts", "**/*.tsx"},
		Exclude: []string{"node_modules"},
	}

	data, _ := json.MarshalIndent(cfg, "", "  ")
	return string(data) + "\n"
}

// GenerateNextTailwindConfig creates a minimal Tailwind v4 config.
func GenerateNextTailwindConfig() string {
	return `import type { Config } from 'tailwindcss';

const config: Config = {
  content: ['./src/**/*.{ts,tsx}'],
  theme: { extend: {} },
  plugins: [],
};

export default config;
`
}

// GenerateNextPostCSS creates a PostCSS config for Tailwind v4.
func GenerateNextPostCSS() string {
	return `const config = {
  plugins: {
    "@tailwindcss/postcss": {},
  },
};
export default config;
`
}

// GenerateNextEnvLocal creates a .env.local file with the backend API URL.
func GenerateNextEnvLocal() string {
	return `# Backend API URL
API_URL=http://localhost:8080
`
}

// GenerateNextGitignore creates a .gitignore for a Next.js project.
func GenerateNextGitignore() string {
	return `node_modules/
.next/
out/
.env*.local
*.tsbuildinfo
next-env.d.ts
`
}

// GenerateNextGlobalCSS creates a global CSS file with Tailwind v4 import.
func GenerateNextGlobalCSS() string {
	return `@import "tailwindcss";
`
}
