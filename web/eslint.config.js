import vueParser from "vue-eslint-parser";
import tsParser from "@typescript-eslint/parser";
import importPlugin from "eslint-plugin-import";

export default [
    {
        ignores: [ "dist/**", "node_modules/**", "src/assets/vendor/**" ],
    },
    {
        files: [ "**/*.{ts,vue}" ],
        languageOptions: {
            parser: vueParser,
            parserOptions: {
                parser: tsParser,
                ecmaVersion: "latest",
                sourceType: "module",
                extraFileExtensions: [ ".vue" ],
            },
        },
        plugins: { import: importPlugin },
        settings: {
            "import/resolver": {
                node: {
                    extensions: [ ".js", ".mjs", ".cjs", ".ts", ".tsx", ".d.ts", ".vue" ],
                },
                typescript: {
                    project: "./tsconfig.app.json",
                },
            },
        },
        rules: {
            "import/no-unresolved": [ "error", { caseSensitive: true } ],
        },
    },
];
