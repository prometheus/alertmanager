import { createContext, type DragEvent, type PropsWithChildren, useContext, useState } from 'react';

import type { Annotation, MatcherDefinition, MatcherPosition } from '@/data/silenceFilters';

import classes from './SilenceTags.module.css';

export type SilenceTagDrag =
  | { kind: 'matcher'; matcher: MatcherDefinition; source?: MatcherPosition }
  | { kind: 'annotation'; annotation: Annotation };

export type ResolveSilenceTagDrop = (tag: SilenceTagDrag) => (() => void) | undefined;

type DragState = {
  tag: SilenceTagDrag | null;
  setTag: (tag: SilenceTagDrag | null) => void;
};

const DragContext = createContext<DragState | null>(null);
const dragType = 'application/x-alertmanager-silence-tag';

const dragEffect = (tag: SilenceTagDrag) =>
  tag.kind === 'matcher' && tag.source ? 'move' : 'copy';

export function SilenceTagDragProvider({ children }: PropsWithChildren) {
  const [tag, setTag] = useState<SilenceTagDrag | null>(null);
  return <DragContext value={{ tag, setTag }}>{children}</DragContext>;
}

export function useSilenceTagDrag(tag?: SilenceTagDrag) {
  const drag = useContext(DragContext);
  if (!drag || !tag) {
    return;
  }
  return {
    draggable: true,
    onDragStart(event: DragEvent<HTMLElement>) {
      // Keep the tag data in this page. Set only a marker in the drag data.
      event.dataTransfer.setData(dragType, tag.kind);
      event.dataTransfer.effectAllowed = dragEffect(tag);
      drag.setTag(tag);
    },
    onDragEnd: () => drag.setTag(null),
  };
}

export function useSilenceTagDrop(resolveDrop: ResolveSilenceTagDrop) {
  const drag = useContext(DragContext);
  const tag = drag?.tag;
  if (!drag || !tag) {
    return;
  }
  const apply = resolveDrop(tag);
  if (!apply) {
    return;
  }
  return {
    className: classes.dropTarget,
    onDragOver(event: DragEvent<HTMLElement>) {
      if (event.dataTransfer.types.includes(dragType)) {
        event.preventDefault();
        event.dataTransfer.dropEffect = dragEffect(tag);
      }
    },
    onDrop(event: DragEvent<HTMLElement>) {
      if (event.dataTransfer.getData(dragType) !== tag.kind) {
        return;
      }
      event.preventDefault();
      drag.setTag(null);
      apply();
    },
  };
}
