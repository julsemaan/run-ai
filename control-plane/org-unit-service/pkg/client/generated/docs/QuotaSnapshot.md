# QuotaSnapshot

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**ControlRevision** | Pointer to **int64** |  | [optional] 
**ControlPhase** | Pointer to **string** |  | [optional] 
**Source** | Pointer to **string** |  | [optional] 
**Epoch** | **string** |  | 
**KubernetesUid** | **string** |  | 
**Generation** | **int64** |  | 
**Pools** | [**[]QuotaSnapshotPool**](QuotaSnapshotPool.md) |  | 

## Methods

### NewQuotaSnapshot

`func NewQuotaSnapshot(epoch string, kubernetesUid string, generation int64, pools []QuotaSnapshotPool, ) *QuotaSnapshot`

NewQuotaSnapshot instantiates a new QuotaSnapshot object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewQuotaSnapshotWithDefaults

`func NewQuotaSnapshotWithDefaults() *QuotaSnapshot`

NewQuotaSnapshotWithDefaults instantiates a new QuotaSnapshot object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetControlRevision

`func (o *QuotaSnapshot) GetControlRevision() int64`

GetControlRevision returns the ControlRevision field if non-nil, zero value otherwise.

### GetControlRevisionOk

`func (o *QuotaSnapshot) GetControlRevisionOk() (*int64, bool)`

GetControlRevisionOk returns a tuple with the ControlRevision field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetControlRevision

`func (o *QuotaSnapshot) SetControlRevision(v int64)`

SetControlRevision sets ControlRevision field to given value.

### HasControlRevision

`func (o *QuotaSnapshot) HasControlRevision() bool`

HasControlRevision returns a boolean if a field has been set.

### GetControlPhase

`func (o *QuotaSnapshot) GetControlPhase() string`

GetControlPhase returns the ControlPhase field if non-nil, zero value otherwise.

### GetControlPhaseOk

`func (o *QuotaSnapshot) GetControlPhaseOk() (*string, bool)`

GetControlPhaseOk returns a tuple with the ControlPhase field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetControlPhase

`func (o *QuotaSnapshot) SetControlPhase(v string)`

SetControlPhase sets ControlPhase field to given value.

### HasControlPhase

`func (o *QuotaSnapshot) HasControlPhase() bool`

HasControlPhase returns a boolean if a field has been set.

### GetSource

`func (o *QuotaSnapshot) GetSource() string`

GetSource returns the Source field if non-nil, zero value otherwise.

### GetSourceOk

`func (o *QuotaSnapshot) GetSourceOk() (*string, bool)`

GetSourceOk returns a tuple with the Source field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSource

`func (o *QuotaSnapshot) SetSource(v string)`

SetSource sets Source field to given value.

### HasSource

`func (o *QuotaSnapshot) HasSource() bool`

HasSource returns a boolean if a field has been set.

### GetEpoch

`func (o *QuotaSnapshot) GetEpoch() string`

GetEpoch returns the Epoch field if non-nil, zero value otherwise.

### GetEpochOk

`func (o *QuotaSnapshot) GetEpochOk() (*string, bool)`

GetEpochOk returns a tuple with the Epoch field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEpoch

`func (o *QuotaSnapshot) SetEpoch(v string)`

SetEpoch sets Epoch field to given value.


### GetKubernetesUid

`func (o *QuotaSnapshot) GetKubernetesUid() string`

GetKubernetesUid returns the KubernetesUid field if non-nil, zero value otherwise.

### GetKubernetesUidOk

`func (o *QuotaSnapshot) GetKubernetesUidOk() (*string, bool)`

GetKubernetesUidOk returns a tuple with the KubernetesUid field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKubernetesUid

`func (o *QuotaSnapshot) SetKubernetesUid(v string)`

SetKubernetesUid sets KubernetesUid field to given value.


### GetGeneration

`func (o *QuotaSnapshot) GetGeneration() int64`

GetGeneration returns the Generation field if non-nil, zero value otherwise.

### GetGenerationOk

`func (o *QuotaSnapshot) GetGenerationOk() (*int64, bool)`

GetGenerationOk returns a tuple with the Generation field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGeneration

`func (o *QuotaSnapshot) SetGeneration(v int64)`

SetGeneration sets Generation field to given value.


### GetPools

`func (o *QuotaSnapshot) GetPools() []QuotaSnapshotPool`

GetPools returns the Pools field if non-nil, zero value otherwise.

### GetPoolsOk

`func (o *QuotaSnapshot) GetPoolsOk() (*[]QuotaSnapshotPool, bool)`

GetPoolsOk returns a tuple with the Pools field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPools

`func (o *QuotaSnapshot) SetPools(v []QuotaSnapshotPool)`

SetPools sets Pools field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


