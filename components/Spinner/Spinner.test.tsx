import React from 'react';
import { render, screen } from '@testing-library/react';
import Spinner from './Spinner';
import '@testing-library/jest-dom';

describe('Spinner Component', () => {
  it('renders correctly when loading is true', () => {
    render(<Spinner loading={true} />);
    const spinner = screen.getByTestId('spinner');
    expect(spinner).toBeInTheDocument();
  });

  it('does not render when loading is false', () => {
    const { container } = render(<Spinner loading={false} />);
    expect(container.firstChild).toBeNull();
  });

  it('applies the correct size class', () => {
    render(<Spinner size="large" />);
    const spinner = screen.getByTestId('spinner');
    expect(spinner.className).toContain('large');
  });

  it('applies the correct color style', () => {
    render(<Spinner color="#ff0000" />);
    const spinner = screen.getByTestId('spinner');
    expect(spinner).toHaveStyle('border-top-color: #ff0000');
  });
});
