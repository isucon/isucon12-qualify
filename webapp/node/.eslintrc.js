module.exports = {
    "env": {
        "browser": true,
        "es2021": true
    },
    "extends": [
        "eslint:recommended",
        "plugin:@typescript-eslint/recommended",
        "prettier"
    ],
    "parser": "@typescript-eslint/parser",
    "parserOptions": {
        "ecmaVersion": "latest",
        "sourceType": "module"
    },
    "plugins": [
        "@typescript-eslint",
    ],
    "rules": {
        "@typescript-eslint/no-explicit-any": 0,
        "@typescript-eslint/no-unused-vars": [
            1, { "argsIgnorePattern": "^_" }
        ]
    }
}
