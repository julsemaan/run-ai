# QuotaSourceUpdateRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Source** | **string** |  | 
**ExpectedRevision** | **int64** |  | 

## Methods

### NewQuotaSourceUpdateRequest

`func NewQuotaSourceUpdateRequest(source string, expectedRevision int64, ) *QuotaSourceUpdateRequest`

NewQuotaSourceUpdateRequest instantiates a new QuotaSourceUpdateRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewQuotaSourceUpdateRequestWithDefaults

`func NewQuotaSourceUpdateRequestWithDefaults() *QuotaSourceUpdateRequest`

NewQuotaSourceUpdateRequestWithDefaults instantiates a new QuotaSourceUpdateRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetSource

`func (o *QuotaSourceUpdateRequest) GetSource() string`

GetSource returns the Source field if non-nil, zero value otherwise.

### GetSourceOk

`func (o *QuotaSourceUpdateRequest) GetSourceOk() (*string, bool)`

GetSourceOk returns a tuple with the Source field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSource

`func (o *QuotaSourceUpdateRequest) SetSource(v string)`

SetSource sets Source field to given value.


### GetExpectedRevision

`func (o *QuotaSourceUpdateRequest) GetExpectedRevision() int64`

GetExpectedRevision returns the ExpectedRevision field if non-nil, zero value otherwise.

### GetExpectedRevisionOk

`func (o *QuotaSourceUpdateRequest) GetExpectedRevisionOk() (*int64, bool)`

GetExpectedRevisionOk returns a tuple with the ExpectedRevision field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExpectedRevision

`func (o *QuotaSourceUpdateRequest) SetExpectedRevision(v int64)`

SetExpectedRevision sets ExpectedRevision field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


