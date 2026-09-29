import type { Element, ElementContent, Root as HastRoot } from 'hast';
import { toString as hastText } from 'hast-util-to-string';
import type { Root } from 'mdast';
import rehypeStringify from 'rehype-stringify';
import remarkRehype from 'remark-rehype';
import { unified } from 'unified';
import { SKIP, visit } from 'unist-util-visit';
import { highlight } from '../highlight';

const h = (tagName: string, properties: Element['properties'], children: ElementContent[] = []): Element => ({
  type: 'element',
  tagName,
  properties,
  children,
});

const text = (value: string) => ({ type: 'text' as const, value });

function rehypeCode() {
  return async (tree: HastRoot) => {
    const jobs: Promise<void>[] = [];
    visit(tree, 'element', (node, index, parent) => {
      if (node.tagName !== 'pre' || !parent || index === undefined) return;
      const code = node.children.find((c): c is Element => c.type === 'element' && c.tagName === 'code');
      if (!code) return;
      const classes = (code.properties.className as string[] | undefined) ?? [];
      const lang = classes.find((c) => c.startsWith('language-'))?.slice(9) ?? '';
      const source = hastText(code).replace(/\n$/, '');

      jobs.push(
        highlight(source, lang).then(({ label, pre }) => {
          parent.children[index] = h('figure', { className: ['code'], dataLang: label }, [
            h('figcaption', { className: ['code-bar'] }, [
              h('span', { className: ['code-lang'] }, [text(label)]),
              h('button', { type: 'button', className: ['code-copy'], dataCopy: '' }, [text('copy')]),
            ]),
            pre,
          ]);
        }),
      );
      return SKIP;
    });
    await Promise.all(jobs);
  };
}

function rehypeHeadings() {
  return (tree: HastRoot) => {
    visit(tree, 'element', (node) => {
      const level = /^h([2-4])$/.exec(node.tagName)?.[1];
      const id = node.properties.id;
      if (!level || typeof id !== 'string') return;
      node.children.unshift(
        h('a', { className: ['hmark'], href: `#${id}`, ariaHidden: 'true', tabIndex: -1 }, [
          text('#'.repeat(Number(level))),
        ]),
      );
    });
  };
}

function rehypeTables() {
  return (tree: HastRoot) => {
    visit(tree, 'element', (node, index, parent) => {
      if (node.tagName !== 'table' || !parent || index === undefined) return;
      parent.children[index] = h('div', { className: ['table-wrap'] }, [node]);
      return SKIP;
    });
  };
}

const processor = unified()
  .use(remarkRehype)
  .use(rehypeCode)
  .use(rehypeHeadings)
  .use(rehypeTables)
  .use(rehypeStringify);

export async function renderMarkdown(tree: Root): Promise<string> {
  const hast = await processor.run(tree);
  return processor.stringify(hast);
}
