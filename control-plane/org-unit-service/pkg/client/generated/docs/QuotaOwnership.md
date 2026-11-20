# QuotaOwnership

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Source** | Pointer to **string** | Target authority. Absent means legacy Kubernetes ownership. | [optional] 
**Phase** | Pointer to **string** |  | [optional] 
**Revision** | Pointer to **int64** |  | [optional] 
**Epoch** | **string** |  | 
**KubernetesUid** | **string** |  | 
**Pools** | [**[]QuotaOwnershipPool**](QuotaOwnershipPool.md) |  | 

## Methods

### NewQuotaOwnership

`func NewQuotaOwnership(epoch string, kubernetesUid string, pools []QuotaOwnershipPool, ) *QuotaOwnership`

NewQuotaOwnership instantiates a new QuotaOwnership object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewQuotaOwnershipWithDefaults

`func NewQuotaOwnershipWithDefaults() *QuotaOwnership`

NewQuotaOwnershipWithDefaults instantiates a new QuotaOwnership object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetSource

`func (o *QuotaOwnership) GetSource() string`

GetSource returns the Source field if non-nil, zero value otherwise.

### GetSourceOk

`func (o *QuotaOwnership) GetSourceOk() (*string, bool)`

GetSourceOk returns a tuple with the Source field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSource

`func (o *QuotaOwnership) SetSource(v string)`

SetSource sets Source field to given value.

### HasSource

`func (o *QuotaOwnership) HasSource() bool`

HasSource returns a boolean if a field has been set.

### GetPhase

`func (o *QuotaOwnership) GetPhase() string`

GetPhase returns the Phase field if non-nil, zero value otherwise.

### GetPhaseOk

`func (o *QuotaOwnership) GetPhaseOk() (*string, bool)`

GetPhaseOk returns a tuple with the Phase field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPhase

`func (o *QuotaOwnership) SetPhase(v string)`

SetPhase sets Phase field to given value.

### HasPhase

`func (o *QuotaOwnership) HasPhase() bool`

HasPhase returns a boolean if a field has been set.

### GetRevision

`func (o *QuotaOwnership) GetRevision() int64`

GetRevision returns the Revision field if non-nil, zero value otherwise.

### GetRevisionOk

`func (o *QuotaOwnership) GetRevisionOk() (*int64, bool)`

GetRevisionOk returns a tuple with the Revision field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRevision

`func (o *QuotaOwnership) SetRevision(v int64)`

SetRevision sets Revision field to given value.

### HasRevision

`func (o *QuotaOwnership) HasRevision() bool`

HasRevision returns a boolean if a field has been set.

### GetEpoch

`func (o *QuotaOwnership) GetEpoch() string`

GetEpoch returns the Epoch field if non-nil, zero value otherwise.

### GetEpochOk

`func (o *QuotaOwnership) GetEpochOk() (*string, bool)`

GetEpochOk returns a tuple with the Epoch field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEpoch

`func (o *QuotaOwnership) SetEpoch(v string)`

SetEpoch sets Epoch field to given value.


### GetKubernetesUid

`func (o *QuotaOwnership) GetKubernetesUid() string`

GetKubernetesUid returns the KubernetesUid field if non-nil, zero value otherwise.

### GetKubernetesUidOk

`func (o *QuotaOwnership) GetKubernetesUidOk() (*string, bool)`

GetKubernetesUidOk returns a tuple with the KubernetesUid field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKubernetesUid

`func (o *QuotaOwnership) SetKubernetesUid(v string)`

SetKubernetesUid sets KubernetesUid field to given value.


### GetPools

`func (o *QuotaOwnership) GetPools() []QuotaOwnershipPool`

GetPools returns the Pools field if non-nil, zero value otherwise.

### GetPoolsOk

`func (o *QuotaOwnership) GetPoolsOk() (*[]QuotaOwnershipPool, bool)`

GetPoolsOk returns a tuple with the Pools field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPools

`func (o *QuotaOwnership) SetPools(v []QuotaOwnershipPool)`

SetPools sets Pools field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


