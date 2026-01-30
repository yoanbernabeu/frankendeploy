import { defineCollection, z } from 'astro:content';
import { glob } from 'astro/loaders';

const docsCollection = defineCollection({
  loader: glob({ pattern: '**/*.{md,mdx}', base: './src/content/docs' }),
  schema: z.object({
    title: z.string(),
    description: z.string().optional(),
    section: z.string().optional(),
    order: z.number().optional(),
    draft: z.boolean().optional().default(false),
  }),
});

export const collections = {
  docs: docsCollection,
};
