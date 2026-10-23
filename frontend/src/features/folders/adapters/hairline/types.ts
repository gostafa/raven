import type { FolderPaper } from '../../domain/types';

declare global {
  interface Window {
    HL: HairlineEngine;
  }
}

type Point = [number, number];
type Camera = object;
type Tween = object;
interface RingPoint {
  u: number;
  v: number;
  nu: number;
  nv: number;
}
interface HairlineEngine {
  Cam: (angle: number, elevation: number, scale: number) => Camera;
  fit: (camera: Camera, points: [number, number, number][], x: number, y: number) => void;
  proj: (camera: Camera) => (x: number, y: number, z: number) => Point;
  fillet: (points: Point[], radii: number[]) => Point[];
  rrect: (x0: number, y0: number, x1: number, y1: number, radius: number, steps: number) => RingPoint[];
  poly: (points: Point[]) => string;
  open: (points: Point[]) => string;
  rad: (degrees: number) => number;
  tween: (value: number) => Tween;
  tset: (tween: Tween, value: number, now: number, delay: number) => void;
  tval: (tween: Tween, now: number) => number;
  tdone: (tween: Tween, now: number) => boolean;
  mk: <K extends keyof SVGElementTagNameMap>(
    tag: K,
    attributes: Record<string, string | number>,
    parent: Element,
  ) => SVGElementTagNameMap[K];
  register: (
    stage: HTMLElement,
    tick: (dt: number, now: number) => boolean,
  ) => { wake: () => void; unregister: () => void };
  pointer: (stage: HTMLElement, handlers: { move: (point: Point) => void; leave: () => void }) => () => void;
  disposer: () => { add: (dispose: () => void) => void; dispose: () => void };
  inject: (root: Document | ShadowRoot) => void;
}
export interface FolderFigureOptions {
  stage: HTMLElement;
  svg: SVGSVGElement;
  read: HTMLElement;
  papers?: readonly FolderPaper[];
  controlled?: boolean;
}
