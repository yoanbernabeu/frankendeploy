export interface NavItem {
  label: string;
  href: string;
  order?: number;
  children?: NavItem[];
}

export interface NavSection {
  title: string;
  items: NavItem[];
}

export const navigation: NavSection[] = [
  {
    title: 'Getting Started',
    items: [
      { label: 'Introduction', href: '/frankendeploy/getting-started/', order: 1 },
      { label: 'Installation', href: '/frankendeploy/installation/', order: 2 },
      { label: 'Quick Start', href: '/frankendeploy/quickstart/', order: 3 },
    ],
  },
  {
    title: 'Guides',
    items: [
      { label: 'Project Configuration', href: '/frankendeploy/guides/configuration/', order: 1 },
      { label: 'Local Development', href: '/frankendeploy/guides/local-development/', order: 2 },
      { label: 'Server Setup', href: '/frankendeploy/guides/server-setup/', order: 3 },
      { label: 'Environment Variables', href: '/frankendeploy/guides/environment-variables/', order: 4 },
      { label: 'Deployment', href: '/frankendeploy/guides/deployment/', order: 5 },
      { label: 'Rollback', href: '/frankendeploy/guides/rollback/', order: 6 },
    ],
  },
  {
    title: 'Configuration',
    items: [
      { label: 'frankendeploy.yaml', href: '/frankendeploy/config/project/', order: 1 },
      { label: 'Global Config', href: '/frankendeploy/config/global/', order: 2 },
    ],
  },
  {
    title: 'CLI Commands',
    items: [
      { label: 'frankendeploy', href: '/frankendeploy/commands/frankendeploy/', order: 1 },
      { label: 'init', href: '/frankendeploy/commands/frankendeploy_init/', order: 2 },
      { label: 'build', href: '/frankendeploy/commands/frankendeploy_build/', order: 3 },
      { label: 'deploy', href: '/frankendeploy/commands/frankendeploy_deploy/', order: 4 },
      { label: 'rollback', href: '/frankendeploy/commands/frankendeploy_rollback/', order: 5 },
      { label: 'logs', href: '/frankendeploy/commands/frankendeploy_logs/', order: 6 },
      { label: 'shell', href: '/frankendeploy/commands/frankendeploy_shell/', order: 7 },
      { label: 'exec', href: '/frankendeploy/commands/frankendeploy_exec/', order: 8 },
      {
        label: 'dev',
        href: '/frankendeploy/commands/frankendeploy_dev/',
        order: 9,
        children: [
          { label: 'up', href: '/frankendeploy/commands/frankendeploy_dev_up/' },
          { label: 'down', href: '/frankendeploy/commands/frankendeploy_dev_down/' },
          { label: 'logs', href: '/frankendeploy/commands/frankendeploy_dev_logs/' },
          { label: 'restart', href: '/frankendeploy/commands/frankendeploy_dev_restart/' },
        ],
      },
      {
        label: 'server',
        href: '/frankendeploy/commands/frankendeploy_server/',
        order: 10,
        children: [
          { label: 'add', href: '/frankendeploy/commands/frankendeploy_server_add/' },
          { label: 'setup', href: '/frankendeploy/commands/frankendeploy_server_setup/' },
          { label: 'list', href: '/frankendeploy/commands/frankendeploy_server_list/' },
          { label: 'status', href: '/frankendeploy/commands/frankendeploy_server_status/' },
          { label: 'remove', href: '/frankendeploy/commands/frankendeploy_server_remove/' },
        ],
      },
      {
        label: 'app',
        href: '/frankendeploy/commands/frankendeploy_app/',
        order: 11,
        children: [
          { label: 'list', href: '/frankendeploy/commands/frankendeploy_app_list/' },
          { label: 'status', href: '/frankendeploy/commands/frankendeploy_app_status/' },
          { label: 'remove', href: '/frankendeploy/commands/frankendeploy_app_remove/' },
        ],
      },
      {
        label: 'env',
        href: '/frankendeploy/commands/frankendeploy_env/',
        order: 12,
        children: [
          { label: 'set', href: '/frankendeploy/commands/frankendeploy_env_set/' },
          { label: 'get', href: '/frankendeploy/commands/frankendeploy_env_get/' },
          { label: 'list', href: '/frankendeploy/commands/frankendeploy_env_list/' },
          { label: 'remove', href: '/frankendeploy/commands/frankendeploy_env_remove/' },
          { label: 'push', href: '/frankendeploy/commands/frankendeploy_env_push/' },
          { label: 'pull', href: '/frankendeploy/commands/frankendeploy_env_pull/' },
        ],
      },
    ],
  },
];

export function getAllPages(): NavItem[] {
  return navigation.flatMap((section) => section.items);
}

function normalizeSlug(slug: string): string {
  return slug.replace(/\.(md|mdx)$/, '').replace(/^\/|\/$/g, '');
}

function normalizeHref(href: string): string {
  return href.replace(/^\/frankendeploy\//, '').replace(/^\/|\/$/g, '');
}

function hrefMatchesSlug(href: string, slug: string): boolean {
  return normalizeHref(href) === normalizeSlug(slug);
}

export function findCurrentSection(slug: string): string | undefined {
  for (const section of navigation) {
    if (section.items.some((item) => hrefMatchesSlug(item.href, slug))) {
      return section.title;
    }
  }
  return undefined;
}

export function findPrevNext(currentSlug: string): { prev?: NavItem; next?: NavItem } {
  const allPages = getAllPages();
  const currentIndex = allPages.findIndex((page) => hrefMatchesSlug(page.href, currentSlug));

  if (currentIndex === -1) return {};

  return {
    prev: currentIndex > 0 ? allPages[currentIndex - 1] : undefined,
    next: currentIndex < allPages.length - 1 ? allPages[currentIndex + 1] : undefined,
  };
}
