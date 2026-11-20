# QuotaManagementSyncStatus

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Phase** | **string** |  | 
**Message** | Pointer to **string** |  | [optional] 
**LastSyncedAt** | Pointer to **NullableTime** |  | [optional] 

## Methods

### NewQuotaManagementSyncStatus

`func NewQuotaManagementSyncStatus(phase string, ) *QuotaManagementSyncStatus`

NewQuotaManagementSyncStatus instantiates a new QuotaManagementSyncStatus object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewQuotaManagementSyncStatusWithDefaults

`func NewQuotaManagementSyncStatusWithDefaults() *QuotaManagementSyncStatus`

NewQuotaManagementSyncStatusWithDefaults instantiates a new QuotaManagementSyncStatus object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetPhase

`func (o *QuotaManagementSyncStatus) GetPhase() string`

GetPhase returns the Phase field if non-nil, zero value otherwise.

### GetPhaseOk

`func (o *QuotaManagementSyncStatus) GetPhaseOk() (*string, bool)`

GetPhaseOk returns a tuple with the Phase field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPhase

`func (o *QuotaManagementSyncStatus) SetPhase(v string)`

SetPhase sets Phase field to given value.


### GetMessage

`func (o *QuotaManagementSyncStatus) GetMessage() string`

GetMessage returns the Message field if non-nil, zero value otherwise.

### GetMessageOk

`func (o *QuotaManagementSyncStatus) GetMessageOk() (*string, bool)`

GetMessageOk returns a tuple with the Message field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMessage

`func (o *QuotaManagementSyncStatus) SetMessage(v string)`

SetMessage sets Message field to given value.

### HasMessage

`func (o *QuotaManagementSyncStatus) HasMessage() bool`

HasMessage returns a boolean if a field has been set.

### GetLastSyncedAt

`func (o *QuotaManagementSyncStatus) GetLastSyncedAt() time.Time`

GetLastSyncedAt returns the LastSyncedAt field if non-nil, zero value otherwise.

### GetLastSyncedAtOk

`func (o *QuotaManagementSyncStatus) GetLastSyncedAtOk() (*time.Time, bool)`

GetLastSyncedAtOk returns a tuple with the LastSyncedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLastSyncedAt

`func (o *QuotaManagementSyncStatus) SetLastSyncedAt(v time.Time)`

SetLastSyncedAt sets LastSyncedAt field to given value.

### HasLastSyncedAt

`func (o *QuotaManagementSyncStatus) HasLastSyncedAt() bool`

HasLastSyncedAt returns a boolean if a field has been set.

### SetLastSyncedAtNil

`func (o *QuotaManagementSyncStatus) SetLastSyncedAtNil(b bool)`

 SetLastSyncedAtNil sets the value for LastSyncedAt to be an explicit nil

### UnsetLastSyncedAt
`func (o *QuotaManagementSyncStatus) UnsetLastSyncedAt()`

UnsetLastSyncedAt ensures that no value is present for LastSyncedAt, not even an explicit nil

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


