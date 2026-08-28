import { docsLoader } from "@astrojs/starlight/loaders";
import { docsSchema } from "@astrojs/starlight/schema";
import { defineCollection } from "astro:content";
import { glob } from "astro/loaders";
import { z } from "astro/zod";
import { ExtendDocsSchema } from "starlight-theme-black/schema";

export const collections = {
  docs: defineCollection({
    loader: docsLoader(),
    schema: docsSchema({ extend: ExtendDocsSchema }),
  }),
  /*
   * What each app can do, as markdown, so the commands on the front page are
   * checked against the command tree by the same test that checks the docs.
   */
  landing: defineCollection({
    loader: glob({ base: "./src/content/landing", pattern: "*.md" }),
    schema: z.object({
      gradient: z.string(),
      href: z.string(),
      order: z.number(),
      summary: z.string(),
      title: z.string(),
    }),
  }),
};
