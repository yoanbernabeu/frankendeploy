import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';
import sitemap from '@astrojs/sitemap';
import tailwind from '@astrojs/tailwind';

export default defineConfig({
  site: 'https://yoanbernabeu.github.io',
  base: '/frankendeploy',
  integrations: [
    tailwind(),
    mdx(),
    sitemap(),
  ],
  markdown: {
    shikiConfig: {
      theme: 'catppuccin-mocha',
      wrap: true,
    },
  },
});
