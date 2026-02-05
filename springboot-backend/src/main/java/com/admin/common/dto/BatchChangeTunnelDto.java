package com.admin.common.dto;

import lombok.Data;
import javax.validation.constraints.NotEmpty;
import javax.validation.constraints.NotNull;
import java.util.List;

@Data
public class BatchChangeTunnelDto {
    
    @NotEmpty(message = "转发ID列表不能为空")
    private List<Long> forwardIds;
    
    @NotNull(message = "目标隧道ID不能为空")
    private Long targetTunnelId;
}
