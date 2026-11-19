# QuotaSnapshotPool

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**NodePoolId** | **string** |  | 
**Gpu** | [**QuotaSnapshotResource**](QuotaSnapshotResource.md) |  | 
**Cpu** | [**QuotaSnapshotResource**](QuotaSnapshotResource.md) |  | 
**Memory** | [**QuotaSnapshotResource**](QuotaSnapshotResource.md) |  | 

## Methods

### NewQuotaSnapshotPool

`func NewQuotaSnapshotPool(nodePoolId string, gpu QuotaSnapshotResource, cpu QuotaSnapshotResource, memory QuotaSnapshotResource, ) *QuotaSnapshotPool`

NewQuotaSnapshotPool instantiates a new QuotaSnapshotPool object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewQuotaSnapshotPoolWithDefaults

`func NewQuotaSnapshotPoolWithDefaults() *QuotaSnapshotPool`

NewQuotaSnapshotPoolWithDefaults instantiates a new QuotaSnapshotPool object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetNodePoolId

`func (o *QuotaSnapshotPool) GetNodePoolId() string`

GetNodePoolId returns the NodePoolId field if non-nil, zero value otherwise.

### GetNodePoolIdOk

`func (o *QuotaSnapshotPool) GetNodePoolIdOk() (*string, bool)`

GetNodePoolIdOk returns a tuple with the NodePoolId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetNodePoolId

`func (o *QuotaSnapshotPool) SetNodePoolId(v string)`

SetNodePoolId sets NodePoolId field to given value.


### GetGpu

`func (o *QuotaSnapshotPool) GetGpu() QuotaSnapshotResource`

GetGpu returns the Gpu field if non-nil, zero value otherwise.

### GetGpuOk

`func (o *QuotaSnapshotPool) GetGpuOk() (*QuotaSnapshotResource, bool)`

GetGpuOk returns a tuple with the Gpu field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGpu

`func (o *QuotaSnapshotPool) SetGpu(v QuotaSnapshotResource)`

SetGpu sets Gpu field to given value.


### GetCpu

`func (o *QuotaSnapshotPool) GetCpu() QuotaSnapshotResource`

GetCpu returns the Cpu field if non-nil, zero value otherwise.

### GetCpuOk

`func (o *QuotaSnapshotPool) GetCpuOk() (*QuotaSnapshotResource, bool)`

GetCpuOk returns a tuple with the Cpu field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCpu

`func (o *QuotaSnapshotPool) SetCpu(v QuotaSnapshotResource)`

SetCpu sets Cpu field to given value.


### GetMemory

`func (o *QuotaSnapshotPool) GetMemory() QuotaSnapshotResource`

GetMemory returns the Memory field if non-nil, zero value otherwise.

### GetMemoryOk

`func (o *QuotaSnapshotPool) GetMemoryOk() (*QuotaSnapshotResource, bool)`

GetMemoryOk returns a tuple with the Memory field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemory

`func (o *QuotaSnapshotPool) SetMemory(v QuotaSnapshotResource)`

SetMemory sets Memory field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


