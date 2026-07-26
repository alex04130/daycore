// Must run before vendor/ds-bundle.js: the design-system bundle is a classic
// IIFE that reads window.React / window.ReactDOM at evaluation time.
import React from 'react';
import * as ReactDOM from 'react-dom';

window.React = React;
window.ReactDOM = ReactDOM;
