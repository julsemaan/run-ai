# QuotaManagement

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Source** | **string** |  | 
**RequestedSource** | **string** |  | 
**Revision** | **int64** |  | 
**CanEditQuotas** | **bool** |  | 
**SyncStatus** | [**QuotaManagementSyncStatus**](QuotaManagementSyncStatus.md) |  | 

## Methods

### NewQuotaManagement

`func NewQuotaManagement(source string, requestedSource string, revision int64, canEditQuotas bool, syncStatus QuotaManagementSyncStatus, ) *QuotaManagement`

NewQuotaManagement instantiates a new QuotaManagement object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewQuotaManagementWithDefaults

`func NewQuotaManagementWithDefaults() *QuotaManagement`

NewQuotaManagementWithDefaults instantiates a new QuotaManagement object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetSource

`func (o *QuotaManagement) GetSource() string`

GetSource returns the Source field if non-nil, zero value otherwise.

### GetSourceOk

`func (o *QuotaManagement) GetSourceOk() (*string, bool)`

GetSourceOk returns a tuple with the Source field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSource

`func (o *QuotaManagement) SetSource(v string)`

SetSource sets Source field to given value.


### GetRequestedSource

`func (o *QuotaManagement) GetRequestedSource() string`

GetRequestedSource returns the RequestedSource field if non-nil, zero value otherwise.

### GetRequestedSourceOk

`func (o *QuotaManagement) GetRequestedSourceOk() (*string, bool)`

GetRequestedSourceOk returns a tuple with the RequestedSource field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRequestedSource

`func (o *QuotaManagement) SetRequestedSource(v string)`

SetRequestedSource sets RequestedSource field to given value.


### GetRevision

`func (o *QuotaManagement) GetRevision() int64`

GetRevision returns the Revision field if non-nil, zero value otherwise.

### GetRevisionOk

`func (o *QuotaManagement) GetRevisionOk() (*int64, bool)`

GetRevisionOk returns a tuple with the Revision field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRevision

`func (o *QuotaManagement) SetRevision(v int64)`

SetRevision sets Revision field to given value.


### GetCanEditQuotas

`func (o *QuotaManagement) GetCanEditQuotas() bool`

GetCanEditQuotas returns the CanEditQuotas field if non-nil, zero value otherwise.

### GetCanEditQuotasOk

`func (o *QuotaManagement) GetCanEditQuotasOk() (*bool, bool)`

GetCanEditQuotasOk returns a tuple with the CanEditQuotas field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanEditQuotas

`func (o *QuotaManagement) SetCanEditQuotas(v bool)`

SetCanEditQuotas sets CanEditQuotas field to given value.


### GetSyncStatus

`func (o *QuotaManagement) GetSyncStatus() QuotaManagementSyncStatus`

GetSyncStatus returns the SyncStatus field if non-nil, zero value otherwise.

### GetSyncStatusOk

`func (o *QuotaManagement) GetSyncStatusOk() (*QuotaManagementSyncStatus, bool)`

GetSyncStatusOk returns a tuple with the SyncStatus field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSyncStatus

`func (o *QuotaManagement) SetSyncStatus(v QuotaManagementSyncStatus)`

SetSyncStatus sets SyncStatus field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


