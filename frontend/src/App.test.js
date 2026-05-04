// Original Create React App test: render App and look for the starter "learn react" link.
// The Nimbus dashboard has replaced that starter screen, so this expectation is outdated.

import { render, screen } from '@testing-library/react';
import App from './App';

test('renders learn react link', () => {
  render(<App />);
  const linkElement = screen.getByText(/learn react/i);
  expect(linkElement).toBeInTheDocument();
});
