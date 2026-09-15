import { render, screen } from '@test-utils';

import { LabelPill } from './LabelPill';

describe('LabelPill', () => {
  // Test basic rendering with required props
  it('renders the label with name and value', () => {
    render(<LabelPill name="severity" value="critical" />);

    // Should display the name and value in "name=value" format
    expect(screen.getByText('severity="critical"')).toBeInTheDocument();
  });

  // Test that component handles special characters in values
  it('renders with special characters in value', () => {
    render(<LabelPill name="path" value="/api/v1/users" />);

    expect(screen.getByText('path="/api/v1/users"')).toBeInTheDocument();
  });

  // Test that remove button callback is triggered when onRemove is provided
  it('calls onRemove handler when remove button is clicked', () => {
    const handleRemove = vi.fn();
    render(<LabelPill name="tag" value="production" withRemoveButton onRemove={handleRemove} />);

    // Find and click the remove button
    const removeButton = screen.getByRole('button', { hidden: true });
    removeButton.click();

    expect(handleRemove).toHaveBeenCalledOnce();
  });

  // Test that the component accepts and applies additional props
  it('applies additional Pill props correctly', () => {
    const { container } = render(
      <LabelPill name="env" value="staging" data-testid="label-pill-component" />
    );

    expect(container.querySelector('[data-testid="label-pill-component"]')).toBeInTheDocument();
  });

  // Test rendering with empty values
  it('renders with empty name or value strings', () => {
    render(<LabelPill name="" value="empty-name" />);

    expect(screen.getByText('="empty-name"')).toBeInTheDocument();
  });
});
