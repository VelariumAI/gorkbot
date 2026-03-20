// Placeholder for a React/Web component Spinner.
// Gorkbot is a Go/BubbleTea terminal application, so a React component
// doesn't fit directly into the TUI, but I am providing it per the exact prompt specs
// (CSS module, default export, props: size, color, loading).

import React from 'react';
import styles from './Spinner.module.css';

interface SpinnerProps {
  size?: 'small' | 'medium' | 'large';
  color?: string;
  loading?: boolean;
}

const Spinner: React.FC<SpinnerProps> = ({
  size = 'medium',
  color = '#000000',
  loading = true,
}) => {
  if (!loading) return null;

  return (
    <div
      className={`${styles.spinner} ${styles[size]}`}
      style={{ borderTopColor: color }}
      data-testid="spinner"
    />
  );
};

export default Spinner;
