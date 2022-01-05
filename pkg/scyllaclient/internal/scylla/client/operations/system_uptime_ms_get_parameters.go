// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"

	strfmt "github.com/go-openapi/strfmt"
)

// NewSystemUptimeMsGetParams creates a new SystemUptimeMsGetParams object
// with the default values initialized.
func NewSystemUptimeMsGetParams() *SystemUptimeMsGetParams {

	return &SystemUptimeMsGetParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewSystemUptimeMsGetParamsWithTimeout creates a new SystemUptimeMsGetParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewSystemUptimeMsGetParamsWithTimeout(timeout time.Duration) *SystemUptimeMsGetParams {

	return &SystemUptimeMsGetParams{

		timeout: timeout,
	}
}

// NewSystemUptimeMsGetParamsWithContext creates a new SystemUptimeMsGetParams object
// with the default values initialized, and the ability to set a context for a request
func NewSystemUptimeMsGetParamsWithContext(ctx context.Context) *SystemUptimeMsGetParams {

	return &SystemUptimeMsGetParams{

		Context: ctx,
	}
}

// NewSystemUptimeMsGetParamsWithHTTPClient creates a new SystemUptimeMsGetParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewSystemUptimeMsGetParamsWithHTTPClient(client *http.Client) *SystemUptimeMsGetParams {

	return &SystemUptimeMsGetParams{
		HTTPClient: client,
	}
}

/*SystemUptimeMsGetParams contains all the parameters to send to the API endpoint
for the system uptime ms get operation typically these are written to a http.Request
*/
type SystemUptimeMsGetParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the system uptime ms get params
func (o *SystemUptimeMsGetParams) WithTimeout(timeout time.Duration) *SystemUptimeMsGetParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the system uptime ms get params
func (o *SystemUptimeMsGetParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the system uptime ms get params
func (o *SystemUptimeMsGetParams) WithContext(ctx context.Context) *SystemUptimeMsGetParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the system uptime ms get params
func (o *SystemUptimeMsGetParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the system uptime ms get params
func (o *SystemUptimeMsGetParams) WithHTTPClient(client *http.Client) *SystemUptimeMsGetParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the system uptime ms get params
func (o *SystemUptimeMsGetParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *SystemUptimeMsGetParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
