import React from 'react';
import { render, screen, waitForElementToBeRemoved } from '@testing-library/react';
import { AddToDashboardButton } from './AddToDashboardButton';
import userEvent from '@testing-library/user-event';
import { openModal } from './__test__/utils';

describe('Add to Dashboard Button', () => {
  it('Opens and closes the modal correctly', async () => {
    render(<AddToDashboardButton queries={[]} visualization="table" onSave={() => Promise.resolve()} />);

    await openModal();

    // waiting on https://github.com/grafana/grafana/pull/45472 to properly test this:
    // expect(screen.getByRole('dialog', { name: 'Add query to dashboard' })).toBeInTheDocument();
    expect(screen.getByText('Add query to dashboard')).toBeInTheDocument();

    userEvent.click(screen.getByRole('button', { name: /cancel/i }));

    // TODO: once https://github.com/grafana/grafana/pull/45472 is merged replace with
    // expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByText('Add query to dashboard')).not.toBeInTheDocument();
  });

  describe('Save to new dashboard', () => {
    it('Does not submit if the form is invalid', async () => {
      const saveMock = jest.fn();

      render(<AddToDashboardButton queries={[]} visualization="table" onSave={saveMock} />);

      await openModal();

      // there shouldn't be any alert in the modal
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();

      const dashboardNameInput = screen.getByRole<HTMLInputElement>('textbox', { name: /dashboard name/i });

      // dashboard name is required
      userEvent.clear(dashboardNameInput);

      userEvent.click(screen.getByRole('button', { name: /save and keep exploring/i }));

      // The error message should appear
      await screen.findByRole('alert');

      // The modal should not get closed
      expect(screen.queryByText('Add query to dashboard')).toBeInTheDocument();

      // Create dashboard API is not invoked
      expect(saveMock).not.toHaveBeenCalled();
    });

    it('Correctly submits if the form is valid', async () => {
      const saveMock = jest.fn();

      render(<AddToDashboardButton queries={[]} visualization="table" onSave={saveMock} />);

      await openModal();

      const dashboardNameInput = screen.getByRole<HTMLInputElement>('textbox', { name: /dashboard name/i });

      userEvent.click(screen.getByRole('button', { name: /save and keep exploring/i }));

      await waitForElementToBeRemoved(() => screen.queryByText('Add query to dashboard'));

      expect(saveMock).toHaveBeenCalledWith(
        {
          dashboardName: dashboardNameInput.value,
          queries: [],
          visualization: 'table',
          folder: expect.objectContaining({ id: 0 }),
        },
        expect.anything()
      );
    });
  });

  describe('Handling API errors', () => {
    it('Correctly sets error related to dashboard name', async () => {
      const saveMock = jest.fn();

      render(<AddToDashboardButton queries={[]} visualization="table" onSave={saveMock} />);

      await openModal();

      // name-exists, triggered when trying to create a dashboard in a folder that already has a dashboard with the same name
      saveMock.mockResolvedValueOnce({ status: 'name-exists', message: 'name exists' });

      userEvent.click(screen.getByRole('button', { name: /save and keep exploring/i }));

      await screen.findByRole('alert');
      expect(await screen.findByRole('alert')).toHaveTextContent('name exists');

      // empty-name, triggered when trying to create a dashboard having an empty name.
      // FE validation usually avoids this use case, but can be triggered by using only whitespaces in
      // dashboard name field
      saveMock.mockResolvedValueOnce({ status: 'empty-name', message: 'empty name' });

      userEvent.click(screen.getByRole('button', { name: /save and keep exploring/i }));

      await screen.findByRole('alert');
      expect(await screen.findByRole('alert')).toHaveTextContent('empty name');

      // name-match, triggered when trying to create a dashboard in a folder that has the same name.
      // it doesn't seem to ever be triggered, but matches the error in
      // https://github.com/grafana/grafana/blob/44f1e381cbc7a5e236b543bc6bd06b00e3152d7f/pkg/models/dashboards.go#L71
      saveMock.mockResolvedValueOnce({ status: 'name-match', message: 'name match' });

      userEvent.click(screen.getByRole('button', { name: /save and keep exploring/i }));

      await screen.findByRole('alert');
      expect(await screen.findByRole('alert')).toHaveTextContent('name match');
    });
  });

  // TODO: handle unknown errors
});
