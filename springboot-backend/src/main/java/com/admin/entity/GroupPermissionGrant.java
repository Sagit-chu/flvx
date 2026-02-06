package com.admin.entity;

import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableId;
import lombok.Data;

import java.io.Serializable;

@Data
public class GroupPermissionGrant implements Serializable {

    private static final long serialVersionUID = 1L;

    @TableId(value = "id", type = IdType.AUTO)
    private Long id;

    private Long userGroupId;

    private Long tunnelGroupId;

    private Long userTunnelId;

    private Integer createdByGroup;

    private Long createdTime;
}
