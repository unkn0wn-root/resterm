import { docHref, site } from '../../site';
import type { Anchors } from './assemble';

const scheme = /^[a-z][a-z\d+.-]*:|^\/\//i;
const docFile = /^(?:\.\/)?([\w.-]+\.md)$/;

// resolveLink turns a link written for GitHub into one for the site. Links
// between docs go to the page that now holds the heading. Other relative links
// point into the repo.
export function resolveLink(url: string, file: string, slug: string, anchors: Anchors): string {
  if (scheme.test(url)) return url;

  const at = url.indexOf('#');
  const path = at < 0 ? url : url.slice(0, at);
  const hash = at < 0 ? '' : url.slice(at + 1);

  const target = path ? docFile.exec(path)?.[1] : file;
  if (!target || !anchors.has(target)) {
    return new URL(url, `${site.repo}/blob/main/docs/`).href;
  }

  const found = anchors.resolve(target, hash);
  if (!found) throw new Error(`docs/${file}: link "${url}" points to a heading that does not exist`);
  if (found.slug === slug) return hash ? `#${hash}` : docHref(slug);
  return docHref(found.slug, found.primary ? undefined : hash);
}
