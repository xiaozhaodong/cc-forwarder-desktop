import test from 'node:test';
import assert from 'node:assert/strict';
import { createModalLifecycleManager } from './modalLifecycle.js';

class FakeElement {
  constructor(documentTarget) {
    this.documentTarget = documentTarget;
    this.connected = true;
  }

  focus() {
    this.documentTarget.activeElement = this;
  }
}

class FakeDocument {
  constructor() {
    this.listeners = new Map();
    this.activeElement = null;
    this.scrollTarget = {
      dataset: {},
      style: { overflow: '' }
    };
    this.body = this.scrollTarget;
  }

  addEventListener(type, listener) {
    const listeners = this.listeners.get(type) || [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }

  removeEventListener(type, listener) {
    const listeners = this.listeners.get(type) || [];
    this.listeners.set(type, listeners.filter((candidate) => candidate !== listener));
  }

  dispatchKey(key) {
    for (const listener of [...(this.listeners.get('keydown') || [])]) {
      listener({ key });
    }
  }

  getElementById(id) {
    return id === 'app-scroll-container' ? this.scrollTarget : null;
  }

  contains(element) {
    return element?.connected === true;
  }

  createElement() {
    return new FakeElement(this);
  }
}

const withFakeDocument = (run) => {
  const previousDocument = globalThis.document;
  const documentTarget = new FakeDocument();
  globalThis.document = documentTarget;
  try {
    return run(documentTarget);
  } finally {
    globalThis.document = previousDocument;
  }
};

test('one Escape closes only the topmost modal', () => withFakeDocument((documentTarget) => {
  const manager = createModalLifecycleManager();
  let lowerCloseCount = 0;
  let upperCloseCount = 0;

  const cleanupLower = manager.activate({
    getOnClose: () => () => { lowerCloseCount += 1; },
    initialFocusElement: documentTarget.createElement()
  });
  let cleanupUpper = () => {};
  cleanupUpper = manager.activate({
    getOnClose: () => () => {
      upperCloseCount += 1;
      cleanupUpper();
    },
    initialFocusElement: documentTarget.createElement()
  });

  documentTarget.dispatchKey('Escape');

  assert.equal(upperCloseCount, 1);
  assert.equal(lowerCloseCount, 0);

  cleanupLower();
}));

test('a topmost modal that refuses to close still shields lower modals', () => withFakeDocument((documentTarget) => {
  const manager = createModalLifecycleManager();
  let lowerCloseCount = 0;
  let upperCloseCount = 0;

  const cleanupLower = manager.activate({
    getOnClose: () => () => { lowerCloseCount += 1; },
    initialFocusElement: documentTarget.createElement()
  });
  const cleanupUpper = manager.activate({
    getOnClose: () => () => { upperCloseCount += 1; },
    initialFocusElement: documentTarget.createElement()
  });

  documentTarget.dispatchKey('Escape');

  assert.equal(upperCloseCount, 1);
  assert.equal(lowerCloseCount, 0);

  cleanupUpper();
  cleanupLower();
}));

test('removing a lower modal out of order keeps focus and Escape on the top modal', () => withFakeDocument((documentTarget) => {
  const manager = createModalLifecycleManager();
  const trigger = documentTarget.createElement();
  const lowerButton = documentTarget.createElement();
  const upperButton = documentTarget.createElement();
  let lowerCloseCount = 0;
  let upperCloseCount = 0;

  trigger.focus();
  const cleanupLower = manager.activate({
    getOnClose: () => () => { lowerCloseCount += 1; },
    initialFocusElement: lowerButton
  });
  const cleanupUpper = manager.activate({
    getOnClose: () => () => { upperCloseCount += 1; },
    initialFocusElement: upperButton
  });

  lowerButton.connected = false;
  cleanupLower();
  documentTarget.dispatchKey('Escape');

  assert.equal(documentTarget.activeElement, upperButton);
  assert.equal(upperCloseCount, 1);
  assert.equal(lowerCloseCount, 0);

  cleanupUpper();
  assert.equal(documentTarget.activeElement, trigger);
}));

test('nested modals keep the app scroll locked until the final modal closes', () => withFakeDocument((documentTarget) => {
  const manager = createModalLifecycleManager();
  documentTarget.scrollTarget.style.overflow = 'auto';

  const cleanupLower = manager.activate({
    getOnClose: () => undefined,
    initialFocusElement: documentTarget.createElement()
  });
  const cleanupUpper = manager.activate({
    getOnClose: () => undefined,
    initialFocusElement: documentTarget.createElement()
  });

  assert.equal(documentTarget.scrollTarget.style.overflow, 'hidden');
  assert.equal(documentTarget.scrollTarget.dataset.scrollLockCount, '2');

  cleanupLower();
  assert.equal(documentTarget.scrollTarget.style.overflow, 'hidden');
  assert.equal(documentTarget.scrollTarget.dataset.scrollLockCount, '1');

  cleanupUpper();
  assert.equal(documentTarget.scrollTarget.style.overflow, 'auto');
  assert.equal(documentTarget.scrollTarget.dataset.scrollLockCount, undefined);
}));

test('closing nested modals restores focus one layer at a time', () => withFakeDocument((documentTarget) => {
  const manager = createModalLifecycleManager();
  const pageTrigger = documentTarget.createElement();
  const lowerButton = documentTarget.createElement();
  const upperButton = documentTarget.createElement();

  pageTrigger.focus();
  const cleanupLower = manager.activate({
    getOnClose: () => undefined,
    initialFocusElement: lowerButton
  });
  const cleanupUpper = manager.activate({
    getOnClose: () => undefined,
    initialFocusElement: upperButton
  });

  assert.equal(documentTarget.activeElement, upperButton);
  cleanupUpper();
  assert.equal(documentTarget.activeElement, lowerButton);
  cleanupLower();
  assert.equal(documentTarget.activeElement, pageTrigger);
}));
