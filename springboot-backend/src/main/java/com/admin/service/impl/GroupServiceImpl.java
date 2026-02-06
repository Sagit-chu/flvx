package com.admin.service.impl;

import com.admin.common.dto.GroupCreateDto;
import com.admin.common.dto.GroupPermissionAssignDto;
import com.admin.common.dto.GroupPermissionDetailDto;
import com.admin.common.dto.GroupUpdateDto;
import com.admin.common.dto.TunnelGroupAssignTunnelsDto;
import com.admin.common.dto.TunnelGroupDetailDto;
import com.admin.common.dto.UserGroupAssignUsersDto;
import com.admin.common.dto.UserGroupDetailDto;
import com.admin.common.lang.R;
import com.admin.entity.GroupPermission;
import com.admin.entity.GroupPermissionGrant;
import com.admin.entity.Tunnel;
import com.admin.entity.TunnelGroup;
import com.admin.entity.TunnelGroupTunnel;
import com.admin.entity.User;
import com.admin.entity.UserGroup;
import com.admin.entity.UserGroupUser;
import com.admin.entity.UserTunnel;
import com.admin.mapper.GroupPermissionGrantMapper;
import com.admin.mapper.GroupPermissionMapper;
import com.admin.mapper.TunnelGroupMapper;
import com.admin.mapper.TunnelGroupTunnelMapper;
import com.admin.mapper.UserGroupMapper;
import com.admin.mapper.UserGroupUserMapper;
import com.admin.service.GroupService;
import com.admin.service.TunnelService;
import com.admin.service.UserService;
import com.admin.service.UserTunnelService;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import javax.annotation.Resource;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.function.Function;
import java.util.stream.Collectors;

@Service
public class GroupServiceImpl implements GroupService {

    @Resource
    private TunnelGroupMapper tunnelGroupMapper;

    @Resource
    private UserGroupMapper userGroupMapper;

    @Resource
    private TunnelGroupTunnelMapper tunnelGroupTunnelMapper;

    @Resource
    private UserGroupUserMapper userGroupUserMapper;

    @Resource
    private GroupPermissionMapper groupPermissionMapper;

    @Resource
    private GroupPermissionGrantMapper groupPermissionGrantMapper;

    @Resource
    private TunnelService tunnelService;

    @Resource
    private UserService userService;

    @Resource
    private UserTunnelService userTunnelService;

    @Override
    public R getTunnelGroups() {
        List<TunnelGroup> groups = tunnelGroupMapper.selectList(new QueryWrapper<TunnelGroup>().orderByAsc("id"));
        List<TunnelGroupTunnel> mappings = tunnelGroupTunnelMapper.selectList(new QueryWrapper<TunnelGroupTunnel>());

        Map<Long, List<TunnelGroupTunnel>> mappingByGroupId = mappings.stream()
                .collect(Collectors.groupingBy(TunnelGroupTunnel::getTunnelGroupId));

        Set<Long> tunnelIds = mappings.stream().map(TunnelGroupTunnel::getTunnelId).collect(Collectors.toSet());
        Map<Long, String> tunnelNameMap = buildTunnelNameMap(tunnelIds);

        List<TunnelGroupDetailDto> result = new ArrayList<>();
        for (TunnelGroup group : groups) {
            TunnelGroupDetailDto dto = new TunnelGroupDetailDto();
            dto.setId(group.getId());
            dto.setName(group.getName());
            dto.setStatus(group.getStatus());
            dto.setCreatedTime(group.getCreatedTime());
            dto.setUpdatedTime(group.getUpdatedTime());

            List<TunnelGroupTunnel> groupMappings = mappingByGroupId.getOrDefault(group.getId(), Collections.emptyList());
            List<Long> ids = groupMappings.stream().map(TunnelGroupTunnel::getTunnelId).collect(Collectors.toList());
            List<String> names = ids.stream().map(tunnelNameMap::get).filter(name -> name != null && !name.isBlank()).collect(Collectors.toList());
            dto.setTunnelIds(ids);
            dto.setTunnelNames(names);
            result.add(dto);
        }
        return R.ok(result);
    }

    @Override
    public R createTunnelGroup(GroupCreateDto dto) {
        String name = dto.getName().trim();
        int count = tunnelGroupMapper.selectCount(new QueryWrapper<TunnelGroup>().eq("name", name));
        if (count > 0) {
            return R.err("隧道分组名称已存在");
        }

        long now = System.currentTimeMillis();
        TunnelGroup group = new TunnelGroup();
        group.setName(name);
        group.setStatus(normalizeStatus(dto.getStatus()));
        group.setCreatedTime(now);
        group.setUpdatedTime(now);
        tunnelGroupMapper.insert(group);
        return R.ok();
    }

    @Override
    public R updateTunnelGroup(GroupUpdateDto dto) {
        TunnelGroup group = tunnelGroupMapper.selectById(dto.getId());
        if (group == null) {
            return R.err("隧道分组不存在");
        }
        String name = dto.getName().trim();
        int count = tunnelGroupMapper.selectCount(new QueryWrapper<TunnelGroup>().eq("name", name).ne("id", dto.getId()));
        if (count > 0) {
            return R.err("隧道分组名称已存在");
        }

        group.setName(name);
        if (dto.getStatus() != null) {
            group.setStatus(dto.getStatus());
        }
        group.setUpdatedTime(System.currentTimeMillis());
        tunnelGroupMapper.updateById(group);
        return R.ok();
    }

    @Override
    public R deleteTunnelGroup(Long id) {
        TunnelGroup group = tunnelGroupMapper.selectById(id);
        if (group == null) {
            return R.err("隧道分组不存在");
        }

        List<GroupPermissionGrant> grants = groupPermissionGrantMapper.selectList(new QueryWrapper<GroupPermissionGrant>().eq("tunnel_group_id", id));
        revokeGrantRecords(grants);
        tunnelGroupTunnelMapper.delete(new QueryWrapper<TunnelGroupTunnel>().eq("tunnel_group_id", id));
        groupPermissionMapper.delete(new QueryWrapper<GroupPermission>().eq("tunnel_group_id", id));
        tunnelGroupMapper.deleteById(id);
        return R.ok();
    }

    @Override
    @Transactional(rollbackFor = Exception.class)
    public R assignTunnelsToGroup(TunnelGroupAssignTunnelsDto dto) {
        TunnelGroup group = tunnelGroupMapper.selectById(dto.getGroupId());
        if (group == null) {
            return R.err("隧道分组不存在");
        }

        Set<Long> tunnelIds = new LinkedHashSet<>(dto.getTunnelIds());
        if (!tunnelIds.isEmpty()) {
            List<Tunnel> tunnels = tunnelService.list(new QueryWrapper<Tunnel>().in("id", tunnelIds));
            if (tunnels.size() != tunnelIds.size()) {
                return R.err("隧道列表中存在无效ID");
            }
        }

        tunnelGroupTunnelMapper.delete(new QueryWrapper<TunnelGroupTunnel>().eq("tunnel_group_id", dto.getGroupId()));

        if (!tunnelIds.isEmpty()) {
            long now = System.currentTimeMillis();
            for (Long tunnelId : tunnelIds) {
                TunnelGroupTunnel relation = new TunnelGroupTunnel();
                relation.setTunnelGroupId(dto.getGroupId());
                relation.setTunnelId(tunnelId);
                relation.setCreatedTime(now);
                tunnelGroupTunnelMapper.insert(relation);
            }
        }

        syncByTunnelGroup(dto.getGroupId());
        return R.ok();
    }

    @Override
    public R getUserGroups() {
        List<UserGroup> groups = userGroupMapper.selectList(new QueryWrapper<UserGroup>().orderByAsc("id"));
        List<UserGroupUser> mappings = userGroupUserMapper.selectList(new QueryWrapper<UserGroupUser>());

        Map<Long, List<UserGroupUser>> mappingByGroupId = mappings.stream()
                .collect(Collectors.groupingBy(UserGroupUser::getUserGroupId));

        Set<Long> userIds = mappings.stream().map(UserGroupUser::getUserId).collect(Collectors.toSet());
        Map<Long, String> userNameMap = buildUserNameMap(userIds);

        List<UserGroupDetailDto> result = new ArrayList<>();
        for (UserGroup group : groups) {
            UserGroupDetailDto dto = new UserGroupDetailDto();
            dto.setId(group.getId());
            dto.setName(group.getName());
            dto.setStatus(group.getStatus());
            dto.setCreatedTime(group.getCreatedTime());
            dto.setUpdatedTime(group.getUpdatedTime());

            List<UserGroupUser> groupMappings = mappingByGroupId.getOrDefault(group.getId(), Collections.emptyList());
            List<Long> ids = groupMappings.stream().map(UserGroupUser::getUserId).collect(Collectors.toList());
            List<String> names = ids.stream().map(userNameMap::get).filter(name -> name != null && !name.isBlank()).collect(Collectors.toList());
            dto.setUserIds(ids);
            dto.setUserNames(names);
            result.add(dto);
        }
        return R.ok(result);
    }

    @Override
    public R createUserGroup(GroupCreateDto dto) {
        String name = dto.getName().trim();
        int count = userGroupMapper.selectCount(new QueryWrapper<UserGroup>().eq("name", name));
        if (count > 0) {
            return R.err("用户分组名称已存在");
        }

        long now = System.currentTimeMillis();
        UserGroup group = new UserGroup();
        group.setName(name);
        group.setStatus(normalizeStatus(dto.getStatus()));
        group.setCreatedTime(now);
        group.setUpdatedTime(now);
        userGroupMapper.insert(group);
        return R.ok();
    }

    @Override
    public R updateUserGroup(GroupUpdateDto dto) {
        UserGroup group = userGroupMapper.selectById(dto.getId());
        if (group == null) {
            return R.err("用户分组不存在");
        }
        String name = dto.getName().trim();
        int count = userGroupMapper.selectCount(new QueryWrapper<UserGroup>().eq("name", name).ne("id", dto.getId()));
        if (count > 0) {
            return R.err("用户分组名称已存在");
        }

        group.setName(name);
        if (dto.getStatus() != null) {
            group.setStatus(dto.getStatus());
        }
        group.setUpdatedTime(System.currentTimeMillis());
        userGroupMapper.updateById(group);
        return R.ok();
    }

    @Override
    public R deleteUserGroup(Long id) {
        UserGroup group = userGroupMapper.selectById(id);
        if (group == null) {
            return R.err("用户分组不存在");
        }

        List<GroupPermissionGrant> grants = groupPermissionGrantMapper.selectList(new QueryWrapper<GroupPermissionGrant>().eq("user_group_id", id));
        revokeGrantRecords(grants);
        userGroupUserMapper.delete(new QueryWrapper<UserGroupUser>().eq("user_group_id", id));
        groupPermissionMapper.delete(new QueryWrapper<GroupPermission>().eq("user_group_id", id));
        userGroupMapper.deleteById(id);
        return R.ok();
    }

    @Override
    @Transactional(rollbackFor = Exception.class)
    public R assignUsersToGroup(UserGroupAssignUsersDto dto) {
        UserGroup group = userGroupMapper.selectById(dto.getGroupId());
        if (group == null) {
            return R.err("用户分组不存在");
        }

        Set<Long> userIds = new LinkedHashSet<>(dto.getUserIds());
        if (!userIds.isEmpty()) {
            List<User> users = userService.list(new QueryWrapper<User>().in("id", userIds).ne("role_id", 0));
            if (users.size() != userIds.size()) {
                return R.err("用户列表中存在无效ID");
            }
        }

        userGroupUserMapper.delete(new QueryWrapper<UserGroupUser>().eq("user_group_id", dto.getGroupId()));

        if (!userIds.isEmpty()) {
            long now = System.currentTimeMillis();
            for (Long userId : userIds) {
                UserGroupUser relation = new UserGroupUser();
                relation.setUserGroupId(dto.getGroupId());
                relation.setUserId(userId);
                relation.setCreatedTime(now);
                userGroupUserMapper.insert(relation);
            }
        }

        syncByUserGroup(dto.getGroupId());
        return R.ok();
    }

    @Override
    public R getGroupPermissions() {
        List<GroupPermission> permissions = groupPermissionMapper.selectList(new QueryWrapper<GroupPermission>().orderByDesc("id"));
        if (permissions.isEmpty()) {
            return R.ok(new ArrayList<GroupPermissionDetailDto>());
        }

        Set<Long> userGroupIds = permissions.stream().map(GroupPermission::getUserGroupId).collect(Collectors.toSet());
        Set<Long> tunnelGroupIds = permissions.stream().map(GroupPermission::getTunnelGroupId).collect(Collectors.toSet());

        Map<Long, String> userGroupNameMap = userGroupMapper.selectList(new QueryWrapper<UserGroup>().in("id", userGroupIds)).stream()
                .collect(Collectors.toMap(UserGroup::getId, UserGroup::getName));
        Map<Long, String> tunnelGroupNameMap = tunnelGroupMapper.selectList(new QueryWrapper<TunnelGroup>().in("id", tunnelGroupIds)).stream()
                .collect(Collectors.toMap(TunnelGroup::getId, TunnelGroup::getName));

        List<GroupPermissionDetailDto> result = new ArrayList<>();
        for (GroupPermission permission : permissions) {
            GroupPermissionDetailDto dto = new GroupPermissionDetailDto();
            dto.setId(permission.getId());
            dto.setUserGroupId(permission.getUserGroupId());
            dto.setTunnelGroupId(permission.getTunnelGroupId());
            dto.setCreatedTime(permission.getCreatedTime());
            dto.setUserGroupName(userGroupNameMap.get(permission.getUserGroupId()));
            dto.setTunnelGroupName(tunnelGroupNameMap.get(permission.getTunnelGroupId()));
            result.add(dto);
        }

        return R.ok(result);
    }

    @Override
    @Transactional(rollbackFor = Exception.class)
    public R assignGroupPermission(GroupPermissionAssignDto dto) {
        UserGroup userGroup = userGroupMapper.selectById(dto.getUserGroupId());
        if (userGroup == null) {
            return R.err("用户分组不存在");
        }
        TunnelGroup tunnelGroup = tunnelGroupMapper.selectById(dto.getTunnelGroupId());
        if (tunnelGroup == null) {
            return R.err("隧道分组不存在");
        }

        int existing = groupPermissionMapper.selectCount(new QueryWrapper<GroupPermission>()
                .eq("user_group_id", dto.getUserGroupId())
                .eq("tunnel_group_id", dto.getTunnelGroupId()));
        if (existing == 0) {
            GroupPermission permission = new GroupPermission();
            permission.setUserGroupId(dto.getUserGroupId());
            permission.setTunnelGroupId(dto.getTunnelGroupId());
            permission.setCreatedTime(System.currentTimeMillis());
            groupPermissionMapper.insert(permission);
        }

        reconcilePermission(dto.getUserGroupId(), dto.getTunnelGroupId());
        return existing > 0 ? R.ok("权限已存在，已完成同步") : R.ok();
    }

    @Override
    public R removeGroupPermission(Long id) {
        GroupPermission permission = groupPermissionMapper.selectById(id);
        if (permission == null) {
            return R.err("权限记录不存在");
        }

        revokeByPermissionPair(permission.getUserGroupId(), permission.getTunnelGroupId());
        groupPermissionMapper.deleteById(id);
        return R.ok();
    }

    private void syncByUserGroup(Long userGroupId) {
        List<GroupPermission> permissions = groupPermissionMapper.selectList(new QueryWrapper<GroupPermission>().eq("user_group_id", userGroupId));
        for (GroupPermission permission : permissions) {
            reconcilePermission(permission.getUserGroupId(), permission.getTunnelGroupId());
        }
    }

    private void syncByTunnelGroup(Long tunnelGroupId) {
        List<GroupPermission> permissions = groupPermissionMapper.selectList(new QueryWrapper<GroupPermission>().eq("tunnel_group_id", tunnelGroupId));
        for (GroupPermission permission : permissions) {
            reconcilePermission(permission.getUserGroupId(), permission.getTunnelGroupId());
        }
    }

    private void reconcilePermission(Long userGroupId, Long tunnelGroupId) {
        Set<Long> userIds = userGroupUserMapper.selectList(new QueryWrapper<UserGroupUser>().eq("user_group_id", userGroupId)).stream()
                .map(UserGroupUser::getUserId)
                .collect(Collectors.toCollection(LinkedHashSet::new));

        Set<Long> tunnelIds = tunnelGroupTunnelMapper.selectList(new QueryWrapper<TunnelGroupTunnel>().eq("tunnel_group_id", tunnelGroupId)).stream()
                .map(TunnelGroupTunnel::getTunnelId)
                .collect(Collectors.toCollection(LinkedHashSet::new));

        Set<String> desiredKeys = new HashSet<>();
        for (Long userId : userIds) {
            for (Long tunnelId : tunnelIds) {
                desiredKeys.add(permissionKey(userId, tunnelId));
            }
        }

        List<GroupPermissionGrant> currentGrants = groupPermissionGrantMapper.selectList(
                new QueryWrapper<GroupPermissionGrant>()
                        .eq("user_group_id", userGroupId)
                        .eq("tunnel_group_id", tunnelGroupId)
        );

        Set<Long> grantUserTunnelIds = currentGrants.stream().map(GroupPermissionGrant::getUserTunnelId).collect(Collectors.toSet());
        Map<Long, UserTunnel> grantUserTunnelMap = new HashMap<>();
        if (!grantUserTunnelIds.isEmpty()) {
            List<UserTunnel> userTunnels = userTunnelService.list(new QueryWrapper<UserTunnel>().in("id", grantUserTunnelIds));
            grantUserTunnelMap = userTunnels.stream().collect(Collectors.toMap(ut -> ut.getId().longValue(), Function.identity()));
        }

        Map<String, UserTunnel> pairUserTunnelMap = new HashMap<>();
        if (!userIds.isEmpty() && !tunnelIds.isEmpty()) {
            List<UserTunnel> pairUserTunnels = userTunnelService.list(new QueryWrapper<UserTunnel>()
                    .in("user_id", userIds)
                    .in("tunnel_id", tunnelIds));
            for (UserTunnel userTunnel : pairUserTunnels) {
                pairUserTunnelMap.putIfAbsent(permissionKey(userTunnel.getUserId().longValue(), userTunnel.getTunnelId().longValue()), userTunnel);
            }
        }

        Set<Long> pairUserTunnelIds = pairUserTunnelMap.values().stream()
                .map(ut -> ut.getId().longValue())
                .collect(Collectors.toSet());
        Map<Long, Long> totalGrantCountMap = buildGrantCountMap(pairUserTunnelIds);
        Set<Long> groupManagedUserTunnelIds = buildGroupManagedUserTunnelIds(pairUserTunnelIds);

        Set<Long> currentGrantUserTunnelIds = currentGrants.stream().map(GroupPermissionGrant::getUserTunnelId).collect(Collectors.toSet());
        if (!desiredKeys.isEmpty()) {
            Map<Long, User> userMap = userService.list(new QueryWrapper<User>().in("id", userIds)).stream()
                    .collect(Collectors.toMap(User::getId, Function.identity()));

            long now = System.currentTimeMillis();
            for (Long userId : userIds) {
                User user = userMap.get(userId);
                if (user == null) {
                    continue;
                }
                for (Long tunnelId : tunnelIds) {
                    String pairKey = permissionKey(userId, tunnelId);
                    UserTunnel userTunnel = pairUserTunnelMap.get(pairKey);
                    if (userTunnel == null) {
                        userTunnel = createGroupManagedUserTunnel(userId, tunnelId, user);
                        Long userTunnelId = resolveUserTunnelId(userTunnel, userId, tunnelId);
                        pairUserTunnelMap.put(pairKey, userTunnel);
                        createGrant(userGroupId, tunnelGroupId, userTunnelId, true, now);
                        currentGrantUserTunnelIds.add(userTunnelId);
                        totalGrantCountMap.put(userTunnelId, 1L);
                        groupManagedUserTunnelIds.add(userTunnelId);
                        continue;
                    }

                    Long userTunnelId = userTunnel.getId().longValue();
                    if (currentGrantUserTunnelIds.contains(userTunnelId)) {
                        continue;
                    }

                    long existingGrantCount = totalGrantCountMap.getOrDefault(userTunnelId, 0L);
                    boolean createdByGroup = groupManagedUserTunnelIds.contains(userTunnelId);
                    createGrant(userGroupId, tunnelGroupId, userTunnelId, createdByGroup, now);
                    currentGrantUserTunnelIds.add(userTunnelId);
                    totalGrantCountMap.put(userTunnelId, existingGrantCount + 1L);
                }
            }
        }

        List<GroupPermissionGrant> staleGrants = new ArrayList<>();
        for (GroupPermissionGrant grant : currentGrants) {
            UserTunnel userTunnel = grantUserTunnelMap.get(grant.getUserTunnelId());
            boolean keep = false;
            if (userTunnel != null) {
                String key = permissionKey(userTunnel.getUserId().longValue(), userTunnel.getTunnelId().longValue());
                keep = desiredKeys.contains(key);
            }

            if (!keep) {
                staleGrants.add(grant);
            }
        }
        revokeGrantRecords(staleGrants);
    }

    private UserTunnel createGroupManagedUserTunnel(Long userId, Long tunnelId, User user) {
        UserTunnel userTunnel = new UserTunnel();
        userTunnel.setUserId(userId.intValue());
        userTunnel.setTunnelId(tunnelId.intValue());
        userTunnel.setStatus(1);
        userTunnel.setInFlow(0L);
        userTunnel.setOutFlow(0L);
        userTunnel.setFlow(user.getFlow());
        userTunnel.setNum(user.getNum());
        userTunnel.setFlowResetTime(user.getFlowResetTime());
        userTunnel.setExpTime(user.getExpTime());
        boolean saved = userTunnelService.save(userTunnel);
        if (!saved) {
            throw new IllegalStateException("创建用户隧道权限失败: userId=" + userId + ", tunnelId=" + tunnelId);
        }
        return userTunnel;
    }

    private Long resolveUserTunnelId(UserTunnel userTunnel, Long userId, Long tunnelId) {
        if (userTunnel.getId() != null) {
            return userTunnel.getId().longValue();
        }

        UserTunnel persisted = userTunnelService.getOne(
                new QueryWrapper<UserTunnel>()
                        .eq("user_id", userId.intValue())
                        .eq("tunnel_id", tunnelId.intValue())
                        .orderByDesc("id")
                        .last("LIMIT 1")
        );
        if (persisted == null || persisted.getId() == null) {
            throw new IllegalStateException("获取用户隧道权限ID失败: userId=" + userId + ", tunnelId=" + tunnelId);
        }
        userTunnel.setId(persisted.getId());
        return persisted.getId().longValue();
    }

    private void createGrant(Long userGroupId, Long tunnelGroupId, Long userTunnelId, boolean createdByGroup, long createdTime) {
        int exists = groupPermissionGrantMapper.selectCount(new QueryWrapper<GroupPermissionGrant>()
                .eq("user_group_id", userGroupId)
                .eq("tunnel_group_id", tunnelGroupId)
                .eq("user_tunnel_id", userTunnelId));
        if (exists > 0) {
            return;
        }

        GroupPermissionGrant grant = new GroupPermissionGrant();
        grant.setUserGroupId(userGroupId);
        grant.setTunnelGroupId(tunnelGroupId);
        grant.setUserTunnelId(userTunnelId);
        grant.setCreatedByGroup(createdByGroup ? 1 : 0);
        grant.setCreatedTime(createdTime);
        groupPermissionGrantMapper.insert(grant);
    }

    private void revokeByPermissionPair(Long userGroupId, Long tunnelGroupId) {
        List<GroupPermissionGrant> grants = groupPermissionGrantMapper.selectList(new QueryWrapper<GroupPermissionGrant>()
                .eq("user_group_id", userGroupId)
                .eq("tunnel_group_id", tunnelGroupId));
        revokeGrantRecords(grants);
    }

    private void revokeGrantRecords(List<GroupPermissionGrant> grants) {
        if (grants == null || grants.isEmpty()) {
            return;
        }

        Set<Long> candidateUserTunnelIds = new HashSet<>();
        Set<Long> groupManagedCandidates = new HashSet<>();
        for (GroupPermissionGrant grant : grants) {
            candidateUserTunnelIds.add(grant.getUserTunnelId());
            if (grant.getCreatedByGroup() != null && grant.getCreatedByGroup() == 1) {
                groupManagedCandidates.add(grant.getUserTunnelId());
            }
            groupPermissionGrantMapper.deleteById(grant.getId());
        }

        Set<Long> stillGrantedUserTunnelIds = groupPermissionGrantMapper.selectList(
                        new QueryWrapper<GroupPermissionGrant>().in("user_tunnel_id", candidateUserTunnelIds)
                ).stream()
                .map(GroupPermissionGrant::getUserTunnelId)
                .collect(Collectors.toSet());

        for (Long userTunnelId : candidateUserTunnelIds) {
            if (!stillGrantedUserTunnelIds.contains(userTunnelId) && groupManagedCandidates.contains(userTunnelId)) {
                userTunnelService.removeUserTunnel(userTunnelId.intValue());
            }
        }
    }

    private Set<Long> buildGroupManagedUserTunnelIds(Set<Long> userTunnelIds) {
        if (userTunnelIds.isEmpty()) {
            return Collections.emptySet();
        }

        return groupPermissionGrantMapper.selectList(new QueryWrapper<GroupPermissionGrant>()
                        .in("user_tunnel_id", userTunnelIds)
                        .eq("created_by_group", 1))
                .stream()
                .map(GroupPermissionGrant::getUserTunnelId)
                .collect(Collectors.toSet());
    }

    private Map<Long, Long> buildGrantCountMap(Set<Long> userTunnelIds) {
        if (userTunnelIds.isEmpty()) {
            return new HashMap<>();
        }

        List<GroupPermissionGrant> grants = groupPermissionGrantMapper.selectList(new QueryWrapper<GroupPermissionGrant>().in("user_tunnel_id", userTunnelIds));
        Map<Long, Long> countMap = new HashMap<>();
        for (GroupPermissionGrant grant : grants) {
            countMap.merge(grant.getUserTunnelId(), 1L, Long::sum);
        }
        return countMap;
    }

    private Map<Long, String> buildTunnelNameMap(Set<Long> tunnelIds) {
        if (tunnelIds.isEmpty()) {
            return Collections.emptyMap();
        }
        return tunnelService.list(new QueryWrapper<Tunnel>().in("id", tunnelIds)).stream()
                .collect(Collectors.toMap(Tunnel::getId, Tunnel::getName));
    }

    private Map<Long, String> buildUserNameMap(Set<Long> userIds) {
        if (userIds.isEmpty()) {
            return Collections.emptyMap();
        }
        return userService.list(new QueryWrapper<User>().in("id", userIds)).stream()
                .collect(Collectors.toMap(User::getId, User::getUser));
    }

    private String permissionKey(Long userId, Long tunnelId) {
        return userId + "_" + tunnelId;
    }

    private int normalizeStatus(Integer status) {
        return status == null ? 1 : status;
    }
}
