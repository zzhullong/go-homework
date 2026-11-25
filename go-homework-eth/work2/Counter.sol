// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

/// @title 一个简单的计数器合约
/// @notice 支持自增、自减和重置，当前计数值可通过 count() 查询
contract Counter {
    // 当前计数值，public 会自动生成 count() 只读方法
    uint256 public count;

    // 计数值发生变化时触发
    event CountChanged(uint256 newCount);

    constructor() {
        count = 0;
    }

    /// @notice 计数加一
    function increment() public {
        count += 1;
        emit CountChanged(count);
    }

    /// @notice 计数减一，计数为 0 时回滚
    function decrement() public {
        require(count > 0, "Counter: count is already zero");
        count -= 1;
        emit CountChanged(count);
    }

    /// @notice 将计数重置为 0
    function reset() public {
        count = 0;
        emit CountChanged(count);
    }
}
