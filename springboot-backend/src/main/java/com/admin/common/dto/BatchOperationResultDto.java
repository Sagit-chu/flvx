package com.admin.common.dto;

import lombok.Data;
import java.util.List;
import java.util.ArrayList;

@Data
public class BatchOperationResultDto {
    
    private int successCount;
    private int failCount;
    private List<FailedItem> failedItems = new ArrayList<>();
    
    @Data
    public static class FailedItem {
        private Long id;
        private String reason;
        
        public FailedItem() {}
        
        public FailedItem(Long id, String reason) {
            this.id = id;
            this.reason = reason;
        }
    }
    
    public void addFailedItem(Long id, String reason) {
        this.failedItems.add(new FailedItem(id, reason));
        this.failCount++;
    }
    
    public void incrementSuccess() {
        this.successCount++;
    }
    
    public boolean isAllSuccess() {
        return failCount == 0;
    }
}
