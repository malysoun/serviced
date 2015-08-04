Given (/^(?:|that )multiple applications and application templates have been added$/) do
    visitApplicationsPage()
    within(@applications_page.services_table) do
        if has_text?("Showing 1 Result")
            # add application
        end
    end
    within(@applications_page.templates_table) do
        if has_text?("Showing 0 Results") || has_text?("Showing 1 Result")
            # add application templates
        end
    end
end

Given (/^(?:|that )Zenoss Core is not added$/) do
    visitApplicationsPage()
    exists = true
    while exists == true do
        within(@applications_page.services_table) do
            exists = checkServiceRows("Zenoss.core")
        end
        removeEntry("Zenoss.core", "service") if exists
    end
end

Given (/^(?:|that )Zenoss Core with the "(.*?)" Deployment ID is added$/) do |id|
    visitApplicationsPage()
    exists = false
    within(@applications_page.services_table) do
        exists = checkServiceRows("Zenoss.core") && checkColumn(id, "Deployment ID")
    end
    addService("Zenoss.core", "default", id) if !exists
    refreshPage()
end

When(/^I am on the applications page$/) do
    visitApplicationsPage()
end

When(/^I click the add Application button$/) do
    @applications_page.addApp_button.click()
end

When(/^I click the add Application Template button$/) do
    @applications_page.addAppTemplate_button.click()
end

When(/^I click the Services Map button$/) do
    @applications_page.servicesMap_button.click()
    waitForPageLoad()
    @servicesMap_page = ServicesMap.new
end

When(/^I fill in the Deployment ID field with "(.*?)"$/) do |deploymentID|
    fillInDeploymentID(deploymentID)
end

When(/^I remove "(.*?)" from the Applications list$/) do |name|
    within(@applications_page.services_table, :text => name) do
        click_link_or_button("Delete")
    end
end

When(/^I remove "(.*?)" from the Application Templates list$/) do |name|
    within(@applications_page.templates_table, :text => name) do
        click_link_or_button("Delete")
    end
end

Then (/^I should see that the application has deployed$/) do
    expect(page).to have_content("App deployed successfully", wait: 120)
    refreshPage() # workaround until apps consistently display on page without refreshing
end

Then (/^I should see that the application has not been deployed$/) do
    expect(page).to have_content("App deploy failed")
end

Then (/^the "Status" column should be sorted with active applications on (top|the bottom)$/) do |order|
    list = @applications_page.status_icons
    for i in 0..(list.size - 2)
        if order == "top"
            # assuming - (ng-isolate-scope down) before + (ng-isolate-scope good)
            expect(list[i][:class]).to be <= list[i + 1][:class]
        else
            expect(list[i][:class]).to be >= list[i + 1][:class]    # assuming + before - before !
        end
    end
end

Then (/^I should see "([^"]*)" in the Services Map$/) do |node|
    within(@servicesMap_page.map) do
        assert_text(node)
    end
end

Then (/^I should see an entry for "(.*?)" in the Applications table$/) do |entry|
    expect(checkServiceRows(entry)).to be true
end

Then (/^I should see an entry for "(.*?)" in the Application Templates table$/) do |entry|
    expect(checkTemplateRows(entry)).to be true
end

def checkServiceRows(row)
    waitForPageLoad()
    found = false
    within(@applications_page.services_table) do
        found = page.has_text?(getTableValue(row))
    end
    return found
end

def checkTemplateRows(row)
    waitForPageLoad()
    found = false
    within(@applications_page.templates_table) do
        found = page.has_text?(getTableValue(row))
    end
    return found
end

def visitApplicationsPage()
    @applications_page = Applications.new
    @applications_page.navbar.applications.click()
    expect(@applications_page).to be_displayed
    waitForPageLoad()
end

def fillInDeploymentID(id)
    @applications_page.deploymentID_field.set getTableValue(id)
end

def addService(name, pool, id)
    @applications_page.addApp_button.click()
    selectOption(name)
    click_link_or_button("Next")
    selectOption(pool)
    click_link_or_button("Next")
    fillInDeploymentID(id)
    click_link_or_button("Deploy")
    expect(page).to have_content("App deployed successfully", wait: 120)
end
