// @ts-check
import starlight from "@astrojs/starlight";
import { defineConfig, fontProviders } from "astro/config";
import starlightLinksValidator from "starlight-links-validator";
import starlightThemeBlack from "starlight-theme-black";

import { origin, repo } from "./site.ts";

export default defineConfig({
  fonts: [
    {
      cssVariable: "--font-inter",
      name: "Inter",
      provider: fontProviders.fontsource(),
    },
  ],
  integrations: [
    starlight({
      customCss: ["./src/styles/proton.css"],
      description:
        "Unofficial, end-to-end encrypted CLI for Proton Mail, Drive, Calendar, Pass and Contacts.",
      editLink: { baseUrl: `${repo}/edit/main/docs/` },
      /*
       * A command line is read, not scrolled. Wrapping keeps a long invocation
       * whole on a narrow screen, which is where most of them are met.
       *
       * A crontab has no grammar to highlight it with, and a schedule is five
       * fields and a command either way.
       */
      expressiveCode: {
        defaultProps: { wrap: true },
        shiki: { langAlias: { cron: "txt" } },
      },
      favicon: "/favicon.svg",
      lastUpdated: true,
      logo: { alt: "", src: "./src/assets/logo.svg" },
      plugins: [
        starlightThemeBlack({
          navLinks: [
            { label: "Docs", link: "/installation/" },
            { label: "Commands", link: "/commands/" },
          ],
        }),
        starlightLinksValidator(),
      ],
      sidebar: [
        {
          items: [
            "installation",
            "getting-started",
            "language",
            "output",
            "references",
          ],
          label: "Start here",
        },
        {
          items: [
            { label: "Command reference", slug: "commands" },
            { items: [{ autogenerate: { directory: "commands" } }], label: "By app" },
            "configuration",
            "scripting",
          ],
          label: "Reference",
        },
        {
          items: ["how-it-works", "human-verification", "limitations"],
          label: "Understanding it",
        },
      ],
      social: [{ href: repo, icon: "github", label: "GitHub" }],
      title: "proton-cli",
    }),
  ],
  site: origin,
  trailingSlash: "always",
});
