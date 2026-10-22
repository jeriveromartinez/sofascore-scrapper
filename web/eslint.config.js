import vueParser from "vue-eslint-parser";
import tsParser from "@typescript-eslint/parser";
import importPlugin from "eslint-plugin-import";

export default [
    {
        ignores: [ "dist/**", "node_modules/**", "src/assets/vendor/**" ],
    },
    {
        // The protobuf types are generated, so the blanket disable comment
        // the generator writes cannot be edited away. Reporting it as an
        // unused directive would make the lint gate fail on a file nobody
        // is allowed to change.
        files: [ "src/proto/**" ],
        linterOptions: { reportUnusedDisableDirectives: "off" },
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
