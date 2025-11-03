import mantine from "eslint-config-mantine";
import tseslint from "typescript-eslint";

export default tseslint.config(
  ...mantine,
  { ignores: ["**/*.{mjs,cjs,js,d.ts,d.mts}", "./.storybook/main.ts"] },
  {
    files: ["**/*.story.tsx"],
    rules: { "no-console": "off" },
  }
);
